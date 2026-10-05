package ethreceipts

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"
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
	filters     []*filterOwner
	finalizer   *finalizer
	mu          sync.Mutex

	pendingReceipts map[receiptKey]*pendingReceipt
	retryMu         sync.Mutex
	deliveryMu      sync.Mutex
	deliveries      map[receiptKey]Receipt
	inFlight        map[receiptKey]struct{}
	claims          map[*filterOwner]common.Hash
}

type pendingReceipt struct {
	receipt     Receipt
	filterer    *filterOwner
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

// Every registration owns a comparable token; the public filter may contain
// slices, maps or functions and may be reused in a later registration.
type filterOwner struct{ Filterer }

func sameFilter(a, b Filterer) bool {
	if reflect.ValueOf(a).Comparable() && reflect.ValueOf(b).Comparable() {
		return a == b
	}
	return reflect.DeepEqual(a, b)
}

func (s *subscriber) Filters() []Filterer {
	s.mu.Lock()
	defer s.mu.Unlock()
	filters := make([]Filterer, len(s.filters))
	for i, owner := range s.filters {
		filters[i] = owner.Filterer
	}
	return filters
}

func (s *subscriber) filterers() []Filterer {
	s.mu.Lock()
	defer s.mu.Unlock()
	filters := make([]Filterer, len(s.filters))
	for i, owner := range s.filters {
		filters[i] = owner
	}
	return filters
}

func (s *subscriber) AddFilter(queries ...FilterQuery) {
	if len(queries) == 0 {
		return
	}
	owners := make([]*filterOwner, len(queries))
	filters := make([]Filterer, len(queries))
	for i, query := range queries {
		filterer, ok := query.(Filterer)
		if !ok {
			panic("ethreceipts: unexpected")
		}
		owners[i] = &filterOwner{filterer}
		filters[i] = owners[i]
	}
	s.mu.Lock()
	if len(s.filters)+len(owners) > maxFiltersPerListener {
		s.listener.log.Warn(fmt.Sprintf("ethreceipts: subscriber has too many filters (%d), ignoring extra", len(s.filters)+len(owners)))
		s.mu.Unlock()
		return
	}
	s.filters = append(s.filters, owners...)
	s.mu.Unlock()
	select {
	case s.listener.registerFiltersCh <- registerFilters{subscriber: s, filters: filters}:
	default:
		s.listener.log.Warn("ethreceipts: listener registerFiltersCh full, dropping filter register")
	}
}

func (s *subscriber) owner(filter Filterer) *filterOwner {
	if owner, ok := filter.(*filterOwner); ok {
		return owner
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, owner := range s.filters {
		if sameFilter(owner.Filterer, filter) {
			return owner
		}
	}
	return nil
}

func (s *subscriber) RemoveFilter(filter Filterer) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	owner := s.owner(filter)
	if owner == nil {
		owner = s.finalizer.findOwner(filter)
	}
	if owner != nil {
		s.retireOwner(owner, true)
	}
}

func (s *subscriber) removeActive(owner *filterOwner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, f := range s.filters {
		if f == owner {
			copy(s.filters[i:], s.filters[i+1:])
			s.filters[len(s.filters)-1] = nil
			s.filters = s.filters[:len(s.filters)-1]
			return
		}
	}
}

// Caller holds deliveryMu. Exhaustion retains queued finals, while explicit
// cancellation and automatic completion release the complete owner lifetime.
func (s *subscriber) retireOwner(owner *filterOwner, cancelFinality bool) {
	s.removeActive(owner)
	if cancelFinality {
		s.finalizer.invalidateOwner(owner)
	}
	if cancelFinality || !s.finalizer.hasOwner(owner) {
		delete(s.claims, owner)
		for key := range s.deliveries {
			if key.owner == owner {
				delete(s.deliveries, key)
			}
		}
	}
	for key := range s.inFlight {
		if key.owner == owner {
			delete(s.inFlight, key)
		}
	}
	s.retryMu.Lock()
	for key := range s.pendingReceipts {
		if key.owner == owner {
			delete(s.pendingReceipts, key)
		}
	}
	s.retryMu.Unlock()
}

func (s *subscriber) exhaustFilter(filter Filterer) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if owner := s.owner(filter); owner != nil {
		s.retireOwner(owner, false)
	}
}

func (s *subscriber) ClearFilters() {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	s.mu.Lock()
	s.filters = nil
	s.mu.Unlock()
	s.finalizer.clear()
	clear(s.claims)
	clear(s.inFlight)
	clear(s.deliveries)
	s.retryMu.Lock()
	clear(s.pendingReceipts)
	s.retryMu.Unlock()
}

// Reserve matches before fetching so concurrent registration and live processing
// cannot select different transactions for one LimitOne filter.
func (s *subscriber) reserve(receipt Receipt, owner *filterOwner) bool {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if !s.hasFilter(owner) || !s.listener.isCurrentBlock(receipt.BlockHash(), receipt.generation) {
		return false
	}
	if s.claims == nil {
		s.claims = make(map[*filterOwner]common.Hash)
	}
	if owner.Options().LimitOne {
		if txn, claimed := s.claims[owner]; claimed && txn != receipt.TransactionHash() {
			return false
		}
		s.claims[owner] = receipt.TransactionHash()
	}
	key := receiptOwner(receipt, owner)
	if _, exists := s.deliveries[key]; exists {
		return false
	}
	if s.inFlight == nil {
		s.inFlight = make(map[receiptKey]struct{})
	}
	if _, exists := s.inFlight[key]; exists {
		return false
	}
	s.retryMu.Lock()
	_, pending := s.pendingReceipts[key]
	s.retryMu.Unlock()
	if pending {
		return false
	}
	s.inFlight[key] = struct{}{}
	return true
}

func (s *subscriber) releaseReservation(receipt Receipt, owner *filterOwner) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	delete(s.inFlight, receiptOwner(receipt, owner))
	if s.finalizer.hasOwner(owner) {
		return
	}
	for key := range s.inFlight {
		if key.owner == owner {
			return
		}
	}
	for key := range s.deliveries {
		if key.owner == owner {
			return
		}
	}
	s.retryMu.Lock()
	defer s.retryMu.Unlock()
	for key := range s.pendingReceipts {
		if key.owner == owner {
			return
		}
	}
	delete(s.claims, owner)
}

func (s *subscriber) hasFilter(owner *filterOwner) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.filters {
		if f == owner {
			return true
		}
	}
	return false
}

// RPC waits happen outside deliveryMu. Rollback, delivery and finalization share
// this lock so an invalidated in-flight receipt cannot be published afterwards.
func (s *subscriber) publish(ctx context.Context, receipt Receipt, owner *filterOwner) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if ctx.Err() != nil || !s.hasFilter(owner) {
		return
	}
	s.listener.receiptMu.Lock()
	defer s.listener.receiptMu.Unlock()
	if !s.listener.currentBlock(receipt.BlockHash(), receipt.generation) {
		return
	}
	key := receiptOwner(receipt, owner)
	if _, delivered := s.deliveries[key]; delivered {
		return
	}
	if owner.Options().LimitOne {
		if txn, claimed := s.claims[owner]; claimed && txn != receipt.TransactionHash() {
			return
		}
	}
	receipt.Filter = owner.Filterer
	receipt.owner = owner
	receipt.Final = s.listener.isBlockFinal(receipt.BlockNumber())
	if !receipt.Final && owner.Options().Finalize {
		s.finalizer.enqueue(owner, receipt, receipt.BlockNumber())
	}
	if s.deliveries == nil {
		s.deliveries = make(map[receiptKey]Receipt)
	}
	s.deliveries[key] = receipt
	s.ch.Send(receipt)
	if owner.Options().LimitOne && (!owner.Options().Finalize || receipt.Final) {
		s.retireOwner(owner, true)
	}
}

func (s *subscriber) rollbackBlock(block blockRef) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	for _, receipt := range s.finalizer.invalidateBlock(block) {
		// Queued finality still owns the delivered receipt after exhaustion,
		// including when finalized-block history has already been pruned.
		key := receiptOwner(receipt, receipt.owner)
		if _, retained := s.deliveries[key]; !retained {
			if s.deliveries == nil {
				s.deliveries = make(map[receiptKey]Receipt)
			}
			s.deliveries[key] = receipt
		}
	}
	s.retryMu.Lock()
	for key := range s.pendingReceipts {
		if key.blockHash == block.hash && key.generation == block.generation {
			delete(s.pendingReceipts, key)
		}
	}
	s.retryMu.Unlock()
	// A custom matcher may require fields that lightweight block receipts lack.
	// Previously published owners already matched, so use their retained data.
	inactive := make(map[*filterOwner]struct{})
	for key, receipt := range s.deliveries {
		if key.blockHash != block.hash || key.generation != block.generation {
			continue
		}
		if !receipt.Reorged {
			receipt.Final = false
			receipt.Reorged = true
			s.deliveries[key] = receipt
			s.ch.Send(receipt)
		}
		if !s.hasFilter(key.owner) {
			inactive[key.owner] = struct{}{}
		}
	}
	// Notify every delivered receipt before retiring an exhausted lifetime.
	for owner := range inactive {
		if !s.finalizer.hasOwner(owner) {
			s.retireOwner(owner, false)
		}
	}
}

func (s *subscriber) rollback(receipt Receipt, owner *filterOwner) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	key := receiptOwner(receipt, owner)
	if !s.hasFilter(owner) {
		return
	}
	if txn, claimed := s.claims[owner]; owner.Options().LimitOne && claimed && txn != receipt.TransactionHash() {
		return
	}
	if retained, ok := s.deliveries[key]; ok {
		if retained.Reorged {
			return
		}
		receipt = retained
	}
	receipt.Filter = owner.Filterer
	receipt.owner = owner
	receipt.Final = false
	receipt.Reorged = true
	if s.deliveries == nil {
		s.deliveries = make(map[receiptKey]Receipt)
	}
	s.deliveries[key] = receipt
	s.ch.Send(receipt)
}

func (s *subscriber) fetchReceipt(ctx context.Context, receipt Receipt) (Receipt, error) {
	l := s.listener
	l.receiptMu.Lock()
	started := l.reorgRevision
	l.receiptMu.Unlock()
	expected := blockRef{receipt.BlockHash(), receipt.generation}
	r, err := l.fetchTransactionReceipt(ctx, receipt.TransactionHash(), true, expected)
	if err != nil {
		return receipt, err
	}
	if expected.hash == (common.Hash{}) {
		l.receiptMu.Lock()
		valid := l.validFetchedBlock(r.BlockHash, blockRef{}, started)
		receipt.generation = l.blockStates[r.BlockHash].generation
		l.receiptMu.Unlock()
		if !valid {
			return receipt, ethereum.NotFound
		}
	}
	receipt.receipt = r
	receipt.logs = r.Logs
	return receipt, nil
}

func (s *subscriber) matchFiltersAndPublish(ctx context.Context, filterers []Filterer, receipts []Receipt) ([]bool, error) {
	oks := make([]bool, len(filterers))
	type match struct {
		receipt Receipt
		owner   *filterOwner
	}
	var matches []match
	owners := make([]*filterOwner, len(filterers))
	for i, filter := range filterers {
		owners[i] = s.owner(filter)
	}
	for _, receipt := range receipts {
		for i, owner := range owners {
			if ctx.Err() != nil {
				return oks, ctx.Err()
			}
			if owner == nil || !s.hasFilter(owner) {
				continue
			}
			matched, err := owner.Match(ctx, receipt)
			if err != nil {
				return oks, superr.New(ErrFilterMatch, err)
			}
			if !matched {
				continue
			}
			oks[i] = true
			if receipt.Reorged {
				s.rollback(receipt, owner)
				continue
			}
			matches = append(matches, match{receipt, owner})
		}
	}
	sem := make(chan struct{}, maxConcurrentReceiptFetches)
	// One owner's fetch failure must not cancel another owner's delivery or retry.
	var g errgroup.Group
	for _, item := range matches {
		if ctx.Err() != nil {
			break
		}
		if !s.reserve(item.receipt, item.owner) {
			continue
		}
		g.Go(func() error {
			defer s.releaseReservation(item.receipt, item.owner)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return ctx.Err()
			}
			receipt := item.receipt
			if receipt.receipt == nil || receipt.receipt.BlockNumber == nil || receipt.receipt.TxHash == (common.Hash{}) || receipt.receipt.BlockHash == (common.Hash{}) {
				completed, err := s.fetchReceipt(ctx, receipt)
				if err != nil {
					if !errors.Is(err, ethereum.NotFound) && ctx.Err() == nil {
						s.addPendingReceipt(receipt, item.owner)
					}
					return superr.Wrap(fmt.Errorf("failed to fetch txn %s receipt", receipt.TransactionHash()), err)
				}
				receipt = completed
			}
			s.publish(ctx, receipt, item.owner)
			return nil
		})
	}
	err := g.Wait()
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	return oks, err
}

func (s *subscriber) finalizeReceipts(blockNum *big.Int) error {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	for _, txn := range s.finalizer.dequeue(blockNum) {
		receipt := txn.receipt
		owner := receipt.owner
		s.listener.receiptMu.Lock()
		if !receipt.Reorged && s.listener.currentBlock(receipt.BlockHash(), receipt.generation) {
			receipt.Final = true
			s.ch.Send(receipt)
			if owner != nil && (owner.Cond().TxnHash != nil || owner.Options().LimitOne) {
				s.retireOwner(owner, true)
			}
		}
		s.listener.receiptMu.Unlock()
		if !s.hasFilter(owner) && !s.finalizer.hasOwner(owner) {
			s.retireOwner(owner, false)
		}
	}
	// Retain mined data until its block is final, for receipt-free rollback.
	for key, receipt := range s.deliveries {
		if receipt.BlockNumber() != nil && s.listener.isBlockFinal(receipt.BlockNumber()) {
			delete(s.deliveries, key)
		}
	}
	return nil
}

func (s *subscriber) addPendingReceipt(receipt Receipt, filterer *filterOwner) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if !s.hasFilter(filterer) {
		return
	}
	s.retryMu.Lock()
	defer s.retryMu.Unlock()

	txnHash := receipt.TransactionHash()
	key := receiptOwner(receipt, filterer)
	if !s.listener.isCurrentBlock(receipt.BlockHash(), receipt.generation) {
		return
	}

	if s.pendingReceipts == nil {
		// lazy init
		s.pendingReceipts = make(map[receiptKey]*pendingReceipt)
	}

	if len(s.pendingReceipts) >= maxPendingReceipts {
		s.listener.log.Error(
			"Pending receipts queue is full, dropping new receipt",
			"txnHash", txnHash.String(),
			"queueSize", len(s.pendingReceipts),
		)
		return
	}

	if _, exists := s.pendingReceipts[key]; exists {
		// already pending, skip
		return
	}

	s.pendingReceipts[key] = &pendingReceipt{
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
			defer s.releaseReservation(p.receipt, p.filterer)

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				// If context is cancelled, release the claim so the item can be retried later.
				s.retryMu.Lock()
				if current, ok := s.pendingReceipts[receiptOwner(p.receipt, p.filterer)]; ok && current == p {
					current.nextRetryAt = time.Now().Add(100 * time.Millisecond) // small delay to avoid immediate retry
				}
				s.retryMu.Unlock()
				return
			}

			// Attempt to fetch the receipt
			txnHash := p.receipt.TransactionHash()
			key := receiptOwner(p.receipt, p.filterer)
			receipt, err := s.fetchReceipt(ctx, p.receipt)

			s.retryMu.Lock()

			// Check if the item still exists and is the same one we claimed.
			currentPending, exists := s.pendingReceipts[key]
			if !exists || currentPending != p {
				s.retryMu.Unlock()
				s.listener.log.Debug("Pending receipt is stale or already processed, skipping retry", "txnHash", txnHash.String())
				return
			}

			if err != nil {
				defer s.retryMu.Unlock()
				if errors.Is(err, ethereum.NotFound) {
					// Transaction genuinely doesn't exist - remove from queue
					delete(s.pendingReceipts, key)
					s.listener.log.Debug("Receipt not found after retry, removing from queue", "txnHash", txnHash.String())
					return
				}

				// Provider error - update retry state directly on the pointer.
				currentPending.attempts++
				if currentPending.attempts >= maxReceiptRetryAttempts {
					delete(s.pendingReceipts, key)
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
			delete(s.pendingReceipts, key)

			attempts := currentPending.attempts
			s.retryMu.Unlock()
			s.publish(ctx, receipt, p.filterer)

			s.listener.log.Info(
				"Successfully fetched receipt after retry",
				"txnHash", txnHash.String(),
				"attempts", attempts,
			)
		}(pending)
	}

	wg.Wait()
}
