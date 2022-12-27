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

	Filters() []Filterer
	AddFilter(filters ...FilterQuery)
	RemoveFilter(filter Filterer)
	ClearFilters()
}

var _ Subscription = &subscriber{}

type subscriber struct {
	listener    *ReceiptListener
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

func (s *subscriber) matchFilters(ctx context.Context, lastBlockNum uint64, filterers []Filterer, receipts []Receipt) ([]bool, error) {
	oks := make([]bool, len(filterers))

	for _, receipt := range receipts {
		for i, filterer := range filterers {
			matched, err := filterer.Match(ctx, receipt)
			if err != nil {
				// TODO: maybe setup ErrMatchFilter and use superr..
				return oks, fmt.Errorf("matchFilter error: %w", err)
			}

			if !matched {
				// skip, not a match
				continue
			}

			// its a match
			oks[i] = true
			receipt := receipt // copy
			receipt.Filter = filterer

			r, err := s.listener.fetchTransactionReceipt(ctx, receipt.Hash())
			if err != nil {
				return oks, err
			}
			receipt.Receipt = r

			logs := make([]types.Log, len(r.Logs))
			for i, log := range r.Logs {
				logs[i] = *log
			}
			receipt.Logs = logs

			// Finality enqueue if filter asked to Finalize
			if filterer.Options().Finalize {
				s.finalizer.enqueue(filterer.FilterID(), receipt, *receipt.BlockNumber)
			}

			// LimitOne will auto unsubscribe now if were not also waiting for finalizer
			if !filterer.Options().Finalize && filterer.Options().LimitOne {
				s.RemoveFilter(receipt.Filter)
			}

			// Broadcast to subscribers
			s.ch.Send(receipt)
		}
	}

	return oks, nil
}
