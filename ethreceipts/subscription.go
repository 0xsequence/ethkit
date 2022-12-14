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
	Subscribe(filters ...Filter)
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
	finalizer   *finalizer
	mu          sync.Mutex
}

type registerFilters struct {
	subscriber *subscriber
	filters    []Filter
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

func (s *subscriber) matchFilters(ctx context.Context, filters []Filter, receipts []Receipt) error {
	for _, receipt := range receipts {
		for _, filter := range filters {
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
				receipt.Receipt = r

				// Finality enqueue..
				// if receipt asked for Finality filter, lets add to array
				f, ok := filter.(FilterTxnHash)
				if ok && f.Finalize {
					s.finalizer.enqueue(receipt, *receipt.BlockNumber)
				}

				// Broadcast to subscribers
				s.sendCh <- receipt

				// auto-unsubscribe if 'Once' is set
				// TODO: .. the issue though is, we need a bit of a finalizer in here..
				// cuz, we want to wait for it to be final too. it could get reorged..
				// switch f := filter.(type) {
				// case FilterTxnHash:
				// 	if f.Once && !receipt.Removed {
				// 		s.RemoveFilter(f)
				// 	}
				// }
			}
		}
	}
	return nil
}
