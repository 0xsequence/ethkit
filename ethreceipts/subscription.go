package ethreceipts

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/goware/channel"
	"github.com/goware/superr"
	"golang.org/x/sync/errgroup"
)

const (
	maxConcurrentReceiptFetches = 10
	maxConcurrentReceiptRetries = 10

	// After this many attempts, we give up retrying a receipt fetch.
	maxReceiptRetryAttempts = 20
)

var (
	maxWaitBetweenRetries = 5 * time.Minute
)

type Subscription interface {
	TransactionReceipt() <-chan Receipt
	Done() <-chan struct{}
	Unsubscribe()

	Filters() []Filterer
	AddFilter(filters ...FilterQuery)
	RemoveFilter(filter Filterer)
	ClearFilters()
}

var _ Subscription = &subscriber{}

type pendingReceipt struct {
	receipt     Receipt
	filterer    Filterer
	attempts    int
	nextRetryAt time.Time
}

type subscriber struct {
	listener    *ReceiptsListener
	ch          channel.Channel[Receipt]
	done        chan struct{}
	unsubscribe func()
	filters     []Filterer
	finalizer   *finalizer
	mu          sync.Mutex

	pendingReceipts map[common.Hash]pendingReceipt
	retryMu         sync.Mutex
}

type registerFilters struct {
	subscriber *subscriber
	filters    []Filterer
}

func (s *subscriber) TransactionReceipt() <-chan Receipt {
	return s.ch.ReadChannel()
}

func (s *subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *subscriber) Unsubscribe() {
	s.unsubscribe()
}

func (s *subscriber) Filters() []Filterer {
	s.mu.Lock()
	defer s.mu.Unlock()
	filters := make([]Filterer, len(s.filters))
	copy(filters, s.filters)
	return filters
}

func (s *subscriber) AddFilter(filterQueries ...FilterQuery) {
	if len(filterQueries) == 0 {
		return
	}

	filters := make([]Filterer, len(filterQueries))
	for i, query := range filterQueries {
		filterer, ok := query.(Filterer)
		if !ok {
			panic("ethreceipts: unexpected")
		}
		filters[i] = filterer
	}

	s.mu.Lock()

	if len(s.filters)+len(filters) > maxFiltersPerListener {
		// too many filters, ignore the extra filter. not ideal, but better than
		// deadlocking
		s.listener.log.Warn(fmt.Sprintf("ethreceipts: subscriber has too many filters (%d), ignoring extra", len(s.filters)+len(filters)))
		// TODO: maybe return an error or force-unsubscribe instead?
		s.mu.Unlock()
		return
	}

	s.filters = append(s.filters, filters...)
	s.mu.Unlock()

	// TODO: maybe add non-blocking push structure like in relayer queue
	select {
	case s.listener.registerFiltersCh <- registerFilters{subscriber: s, filters: filters}:
		// ok
	default:
		s.listener.log.Warn("ethreceipts: listener registerFiltersCh full, dropping filter register")
	}
}

func (s *subscriber) RemoveFilter(filter Filterer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, f := range s.filters {
		if f == filter {
			s.filters = append(s.filters[:i], s.filters[i+1:]...)
			return
		}
	}
}

func (s *subscriber) ClearFilters() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filters = s.filters[:0]
}

func (s *subscriber) matchFilters(ctx context.Context, filterers []Filterer, receipts []Receipt) ([]bool, error) {
	oks := make([]bool, len(filterers))

	// Collect matches that need receipt fetching
	type matchedReceipt struct {
		receipt     Receipt
		filtererIdx int
		filterer    Filterer
	}
	var toFetch []matchedReceipt
	var mu sync.Mutex

	// First pass: find all matches
	for _, receipt := range receipts {
		for i, filterer := range filterers {
			matched, err := filterer.Match(ctx, receipt)
			if err != nil {
				return oks, superr.New(ErrFilterMatch, err)
			}

			if !matched {
				// skip, not a match
				continue
			}

			// its a match
			oks[i] = true

			if !receipt.Reorged {
				toFetch = append(toFetch, matchedReceipt{
					receipt:     receipt,
					filtererIdx: i,
					filterer:    filterer,
				})
			}
		}
	}

	if len(toFetch) == 0 {
		return oks, nil
	}

	// Fetch receipts concurrently
	sem := make(chan struct{}, maxConcurrentReceiptFetches)
	g, gctx := errgroup.WithContext(ctx)

	for _, item := range toFetch {
		item := item // capture loop variable
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-gctx.Done():
				return gctx.Err()
			}

			// Fetch transaction receipt
			r, err := s.listener.fetchTransactionReceipt(gctx, item.receipt.TransactionHash(), true)
			if err != nil {
				if errors.Is(err, ethereum.NotFound) {
					// not found, don't retry
					return superr.Wrap(fmt.Errorf("txn %s not found", item.receipt.TransactionHash()), err)
				}

				// might be a provider issue, add to pending receipts for retry
				s.addPendingReceipt(item.receipt, item.filterer)
				return superr.Wrap(fmt.Errorf("failed to fetch txn %s receipt", item.receipt.TransactionHash()), err)
			}

			// Update receipt with fetched data
			item.receipt.receipt = r
			item.receipt.logs = r.Logs
			item.receipt.Filter = item.filterer

			// Finality enqueue if filter asked to Finalize, and receipt isn't already final
			if !item.receipt.Final && item.filterer.Options().Finalize {
				s.finalizer.enqueue(item.filterer.FilterID(), item.receipt, item.receipt.BlockNumber())
			}

			// LimitOne will auto unsubscribe now if were not also waiting for finalizer,
			// and if the returned txn isn't one that has been reorged
			//
			// NOTE: when Finalize is set, we don't want to remove this filter until the txn finalizes,
			// because its possible that it can reorg and we have to fetch it again after being re-mined.
			// So we only remove the filter now if the filter finalizer isn't used, otherwise the
			// finalizer will remove the LimitOne filter
			toFinalize := item.filterer.Options().Finalize && !item.receipt.Final
			if item.filterer.Options().LimitOne && !toFinalize {
				s.RemoveFilter(item.receipt.Filter)
			}

			// Check if receipt is already final, in case comes from cache when
			// previously final was not toggled.
			if s.listener.isBlockFinal(item.receipt.BlockNumber()) {
				item.receipt.Final = true
			}

			// Broadcast to subscribers (needs mutex as multiple goroutines may send)
			mu.Lock()
			s.ch.Send(item.receipt)
			mu.Unlock()

			return nil
		})
	}

	// Wait for all fetches to complete
	if err := g.Wait(); err != nil {
		return oks, err
	}

	return oks, nil
}

func (s *subscriber) finalizeReceipts(blockNum *big.Int) error {
	// check subscriber finalizer
	finalizer := s.finalizer
	if finalizer.len() == 0 {
		return nil
	}

	finalTxns := finalizer.dequeue(blockNum)
	if len(finalTxns) == 0 {
		// no matching txns which have been finalized
		return nil
	}

	// dispatch to subscriber finalized receipts
	for _, x := range finalTxns {
		if x.receipt.Reorged {
			// for removed receipts, just skip
			continue
		}

		// mark receipt as final, and send the receipt payload to the subscriber
		x.receipt.Final = true

		// send to the subscriber
		s.ch.Send(x.receipt)

		// Automatically remove filters for finalized txn hashes, as they won't come up again.
		filter := x.receipt.Filter
		if filter != nil && (filter.Cond().TxnHash != nil || filter.Options().LimitOne) {
			s.RemoveFilter(filter)
		}
	}

	return nil
}

func (s *subscriber) addPendingReceipt(receipt Receipt, filterer Filterer) {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()

	txHash := receipt.TransactionHash()

	if s.pendingReceipts == nil {
		// lazy init
		s.pendingReceipts = make(map[common.Hash]pendingReceipt)
	}

	if _, exists := s.pendingReceipts[txHash]; exists {
		// already pending, skip
		return
	}

	s.pendingReceipts[receipt.TransactionHash()] = pendingReceipt{
		receipt:     receipt,
		filterer:    filterer,
		attempts:    1,
		nextRetryAt: time.Now().Add(1 * time.Second), // first retry after 1s
	}

	s.listener.log.Info(fmt.Sprintf("ethreceipts: added pending receipt for txn %s", txHash.Hex()))
}

func (s *subscriber) retryPendingReceipts(ctx context.Context) {
	s.retryMu.Lock()

	// Collect receipts that are due for retry
	var toRetry []pendingReceipt
	now := time.Now()

	for _, pending := range s.pendingReceipts {
		if now.After(pending.nextRetryAt) {
			toRetry = append(toRetry, pending)
		}
	}
	s.retryMu.Unlock()

	if len(toRetry) == 0 {
		return
	}

	s.listener.log.Info(fmt.Sprintf("ethreceipts: retrying %d pending receipts", len(toRetry)))

	// Process retries concurrently with bounded parallelism
	sem := make(chan struct{}, maxConcurrentReceiptRetries)
	var wg sync.WaitGroup

	for _, pending := range toRetry {
		wg.Add(1)
		go func(p pendingReceipt) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			// Attempt to fetch the receipt
			txHash := p.receipt.TransactionHash()
			r, err := s.listener.fetchTransactionReceipt(ctx, txHash, true)

			s.retryMu.Lock()
			defer s.retryMu.Unlock()

			if err != nil {

				if errors.Is(err, ethereum.NotFound) {
					// Transaction genuinely doesn't exist - remove from queue
					delete(s.pendingReceipts, txHash)
					s.listener.log.Debug("Receipt not found after retry, removing from queue",
						"txHash", txHash.String())
					return
				}

				// Provider error - update retry state
				p.attempts++

				// Check if max attempts reached
				if p.attempts >= maxReceiptRetryAttempts {
					// This will definitely remove the pending receipt, if it arrives
					// later subscribers won't get notified.
					delete(s.pendingReceipts, txHash)
					s.listener.log.Error("Failed to fetch receipt after max retries",
						"txHash", txHash.String(),
						"attempts", p.attempts,
						"error", err)

					// TODO: perhaps we should close the subscription here as we failed
					// to deliver a receipt after many attempts?
					return
				}

				// Exponential backoff for next retry
				backoff := time.Duration(1<<uint(p.attempts)) * time.Second
				if backoff > maxWaitBetweenRetries {
					backoff = maxWaitBetweenRetries
				}

				p.nextRetryAt = time.Now().Add(backoff)
				s.pendingReceipts[txHash] = p

				s.listener.log.Debug("Receipt fetch failed, will retry",
					"txHash", txHash.String(),
					"attempt", p.attempts,
					"nextRetryIn", backoff,
				)
				return
			}

			// Remove from pending list
			delete(s.pendingReceipts, txHash)

			// Update receipt with fetched data
			p.receipt.receipt = r
			p.receipt.logs = r.Logs
			p.receipt.Filter = p.filterer

			// Check finality
			if s.listener.isBlockFinal(r.BlockNumber) {
				p.receipt.Final = true
			}

			// Handle finalization queue if needed
			if !p.receipt.Final && p.filterer.Options().Finalize {
				s.finalizer.enqueue(p.filterer.FilterID(), p.receipt, r.BlockNumber)
			}

			// Handle LimitOne filter removal
			toFinalize := p.filterer.Options().Finalize && !p.receipt.Final
			if p.filterer.Options().LimitOne && !toFinalize {
				s.RemoveFilter(p.filterer)
			}

			// Send to subscriber
			s.ch.Send(p.receipt)

			s.listener.log.Info("Successfully fetched receipt after retry",
				"txHash", txHash.String(),
				"attempts", p.attempts)
		}(pending)
	}

	wg.Wait()
}
