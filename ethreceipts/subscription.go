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

	// Maximum number of pending receipts to track for retries.
	maxPendingReceipts = 5000
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

type subscriber struct {
	listener    *ReceiptsListener
	ch          channel.Channel[Receipt]
	done        chan struct{}
	unsubscribe func()
	filters     []Filterer
	finalizer   *finalizer
	mu          sync.Mutex

	pendingReceipts map[common.Hash]*pendingReceipt
	retryMu         sync.Mutex
}

type pendingReceipt struct {
	receipt     Receipt
	filterer    Filterer
	attempts    int
	nextRetryAt time.Time
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
				return superr.Wrap(fmt.Errorf("failed to fetch txn %s receipt due to node issue", item.receipt.TransactionHash()), err)
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
			s.ch.Send(item.receipt)

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

	txnHash := receipt.TransactionHash()

	if s.pendingReceipts == nil {
		// lazy init
		s.pendingReceipts = make(map[common.Hash]*pendingReceipt)
	}

	if len(s.pendingReceipts) >= maxPendingReceipts {
		s.listener.log.Error(
			"Pending receipts queue is full, dropping new receipt",
			"txnHash", txnHash.String(),
			"queueSize", len(s.pendingReceipts),
		)
		return
	}

	if _, exists := s.pendingReceipts[txnHash]; exists {
		// already pending, skip
		return
	}

	s.pendingReceipts[txnHash] = &pendingReceipt{
		receipt:     receipt,
		filterer:    filterer,
		attempts:    1,
		nextRetryAt: time.Now().Add(1 * time.Second), // first retry after 1s
	}

	s.listener.log.Info(fmt.Sprintf("ethreceipts: added pending receipt for txn %s", txnHash.Hex()))
}

func (s *subscriber) retryPendingReceipts(ctx context.Context) {
	s.retryMu.Lock()

	// Create a snapshot of receipts that are due for retry
	var toRetry []*pendingReceipt
	now := time.Now()

	for _, pending := range s.pendingReceipts {
		if now.After(pending.nextRetryAt) {
			// Claim this item for retry by pushing the nextRetryAt into the future,
			// this prevents other concurrent retryPendingReceipts calls from picking
			// it up.
			pending.nextRetryAt = time.Now().Add(10 * time.Minute)
			toRetry = append(toRetry, pending)
		}
	}
	s.retryMu.Unlock()

	if len(toRetry) == 0 {
		return
	}

	// Log warning here, as we treat any need for retrying to fetch a receipt as a warning,
	// and indicates some kind of node/provider issue.
	s.listener.log.Warn(fmt.Sprintf("ethreceipts: retrying %d pending receipts", len(toRetry)))

	// Collect receipts that are due for retry
	sem := make(chan struct{}, maxConcurrentReceiptRetries)
	var wg sync.WaitGroup

	for _, pending := range toRetry {
		wg.Add(1)
		go func(p *pendingReceipt) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				// If context is cancelled, release the claim so the item can be retried later.
				s.retryMu.Lock()
				if current, ok := s.pendingReceipts[p.receipt.TransactionHash()]; ok && current == p {
					current.nextRetryAt = time.Now().Add(100 * time.Millisecond) // small delay to avoid immediate retry
				}
				s.retryMu.Unlock()
				return
			}

			// Attempt to fetch the receipt
			txnHash := p.receipt.TransactionHash()
			r, err := s.listener.fetchTransactionReceipt(ctx, txnHash, true)

			s.retryMu.Lock()
			defer s.retryMu.Unlock()

			// Check if the item still exists and is the same one we claimed.
			currentPending, exists := s.pendingReceipts[txnHash]
			if !exists || currentPending != p {
				s.listener.log.Debug("Pending receipt is stale or already processed, skipping retry", "txnHash", txnHash.String())
				return
			}

			if err != nil {
				if errors.Is(err, ethereum.NotFound) {
					// Transaction genuinely doesn't exist - remove from queue
					delete(s.pendingReceipts, txnHash)
					s.listener.log.Debug("Receipt not found after retry, removing from queue", "txnHash", txnHash.String())
					return
				}

				// Provider error - update retry state directly on the pointer.
				currentPending.attempts++
				if currentPending.attempts >= maxReceiptRetryAttempts {
					delete(s.pendingReceipts, txnHash)
					s.listener.log.Error(
						"Failed to fetch receipt after max retries",
						"txnHash", txnHash.String(),
						"attempts", currentPending.attempts,
						"error", err,
					)
					// TODO: perhaps we should close the subscription here as we failed
					// to deliver a receipt after many attempts?
					return
				}

				// Exponential backoff for next retry
				backoff := time.Duration(1<<uint(currentPending.attempts)) * time.Second
				if backoff > maxWaitBetweenRetries {
					backoff = maxWaitBetweenRetries
				}
				currentPending.nextRetryAt = time.Now().Add(backoff)

				s.listener.log.Debug(
					"Receipt fetch failed, will retry",
					"txnHash", txnHash.String(),
					"attempt", currentPending.attempts,
					"nextRetryIn", backoff,
				)
				return
			}

			// Remove from pending list
			delete(s.pendingReceipts, txnHash)

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

			s.listener.log.Info(
				"Successfully fetched receipt after retry",
				"txnHash", txnHash.String(),
				"attempts", currentPending.attempts,
			)
		}(pending)
	}

	wg.Wait()
}
