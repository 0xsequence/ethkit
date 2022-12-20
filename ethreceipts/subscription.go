package ethreceipts

import (
	"context"
	"fmt"
	"sync"

	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/goware/channel"
)

type Subscription interface {
	TransactionReceipt() <-chan Receipt
	Done() <-chan struct{}
	Unsubscribe()

	Filters() []Filter
	Subscribe(filters ...Filter)
	ClearFilter(filter Filter)
	ClearFilters()
}

var _ Subscription = &subscriber{}

type subscriber struct {
	listener    *ReceiptListener
	ch          channel.Channel[Receipt]
	done        chan struct{}
	unsubscribe func()
	filters     []Filter
	finalizer   *finalizer
	mu          sync.Mutex
}

type registerFilters struct {
	subscriber *subscriber
	filters    []Filter
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

func (s *subscriber) Filters() []Filter {
	s.mu.Lock()
	defer s.mu.Unlock()
	filters := make([]Filter, len(s.filters))
	copy(filters, s.filters)
	return filters
}

func (s *subscriber) Subscribe(filters ...Filter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filters = append(s.filters, filters...)
	s.listener.registerFiltersCh <- registerFilters{subscriber: s, filters: filters}

}

func (s *subscriber) ClearFilter(filter Filter) {
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

func (s *subscriber) matchFilters(ctx context.Context, filters []Filter, receipts []Receipt) error {
	for _, receipt := range receipts {
		for _, filter := range filters {
			ok, err := filter.Match(ctx, receipt)
			if err != nil {
				// TODO: maybe setup ErrMatchFilter and use superr..
				return fmt.Errorf("matchFilter error: %w", err)
			}

			if !ok {
				// skip, not a match
				continue
			}

			// its a match
			receipt := receipt // copy
			receipt.Filter = filter

			r, err := s.listener.fetchTransactionReceipt(ctx, receipt.Hash())
			if err != nil {
				return err
			}
			receipt.Receipt = r

			logs := make([]types.Log, len(r.Logs))
			for i, log := range r.Logs {
				logs[i] = *log
			}
			receipt.Logs = logs

			// Finality enqueue if filter asked to Finalize
			cond, _ := filter.(*FilterCond)
			if cond != nil && cond.FilterOpts.Finalize {
				s.finalizer.enqueue(filter.GetID(), receipt, *receipt.BlockNumber)
			}

			// LimitOne will auto unsubscribe now if were not also waiting for finalizer
			if cond != nil && !cond.FilterOpts.Finalize && cond.FilterOpts.LimitOne {
				s.ClearFilter(receipt.Filter)
			}

			// Broadcast to subscribers
			s.ch.Send(receipt)
		}
	}
	return nil
}
