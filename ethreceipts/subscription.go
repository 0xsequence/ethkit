package ethreceipts

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/goware/channel"
	"github.com/goware/superr"
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

	s.mu.Lock()
	defer s.mu.Unlock()

	filters := make([]Filterer, len(filterQueries))
	for i, query := range filterQueries {
		filterer, ok := query.(Filterer)
		if !ok {
			panic("ethreceipts: unexpected")
		}
		filters[i] = filterer
	}

	s.filters = append(s.filters, filters...)

	// TODO: maybe add non-blocking push structure like in relayer queue
	s.listener.registerFiltersCh <- registerFilters{subscriber: s, filters: filters}
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
			receipt := receipt // copy
			receipt.Filter = filterer

			// fetch transaction receipt if its not been marked as reorged
			if !receipt.Reorged {
				r, err := s.listener.fetchTransactionReceipt(ctx, receipt.TransactionHash(), true)
				if err != nil {
					// TODO: is this fine to return error..? its a bit abrupt.
					// Options are to set FailedFetch bool on the Receipt, and still send to s.ch,
					// or just log the error and continue to the next receipt
					return oks, superr.Wrap(fmt.Errorf("failed to fetch txn %s receipt", receipt.TransactionHash()), err)
				}
				receipt.receipt = r
				receipt.logs = r.Logs
			}

			// Finality enqueue if filter asked to Finalize, and receipt isn't already final
			if !receipt.Reorged && !receipt.Final && filterer.Options().Finalize {
				s.finalizer.enqueue(filterer.FilterID(), receipt, receipt.BlockNumber())
			}

			// LimitOne will auto unsubscribe now if were not also waiting for finalizer,
			// and if the returned txn isn't one that has been reorged
			//
			// NOTE: when Finalize is set, we don't want to remove this filter until the txn finalizes,
			// because its possible that it can reorg and we have to fetch it again after being re-mined.
			// So we only remove the filter now if the filter finalizer isn't used, otherwise the
			// finalizer will remove the LimitOne filter
			toFinalize := filterer.Options().Finalize && !receipt.Final
			if !receipt.Reorged && filterer.Options().LimitOne && !toFinalize {
				s.RemoveFilter(receipt.Filter)
			}

			// Check if receipt is already final, in case comes from cache when
			// previously final was not toggled.
			if s.listener.isBlockFinal(receipt.BlockNumber()) {
				receipt.Final = true
			}

			// Broadcast to subscribers
			s.ch.Send(receipt)
		}
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
