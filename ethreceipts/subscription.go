package ethreceipts

import (
	"context"
	"sync"
)

type Subscription interface {
	TransactionReceipt() <-chan Receipt
	Done() <-chan struct{}
	Unsubscribe()

	Filters() []Filter
	AddFilter(filter Filter)
	RemoveFilter(filter Filter)
	ClearFilters()
}

var _ Subscription = &subscriber{}

type subscriber struct {
	listener    *ReceiptListener
	ch          <-chan Receipt
	sendCh      chan<- Receipt
	done        chan struct{}
	unsubscribe func()
	filters     []Filter
	mu          sync.Mutex
}

func (s *subscriber) TransactionReceipt() <-chan Receipt {
	return s.ch
}

func (s *subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *subscriber) Unsubscribe() {
	s.unsubscribe()
}

func (s *subscriber) Filters() []Filter {
	return s.filters
}

func (s *subscriber) AddFilter(filter Filter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filters = append(s.filters, filter)
}

func (s *subscriber) RemoveFilter(filter Filter) {
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

func (s *subscriber) processFilters(ctx context.Context, receipts []Receipt) error {
	for _, receipt := range receipts {
		for _, filter := range s.filters {
			ok, err := filter.Match(ctx, receipt)
			if err != nil {
				// TODO: lets just log the error here for the filter name
				panic("wee")
			}

			// its a match
			if ok {
				receipt.Filter = filter

				// TODO: if Filter.FetchReceipt.. then lets fetch it and set it, etc..

				// TODO: for now we just fetch all receipts.. later, lets turn this off..
				r, err := s.listener.fetchTransactionReceipt(ctx, receipt.Hash())
				if err != nil {
					return err
				}
				// TODO: naming..
				receipt.Receipt = r

				s.sendCh <- receipt
			}
		}
	}
	return nil
}
