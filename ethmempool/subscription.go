package ethmempool

type Subscription interface {
	PendingTransactionHash() <-chan string
	Done() <-chan struct{}
	Unsubscribe()
}

type subscriber struct {
	ch               chan string
	done             chan struct{}
	unsubscribe      func()
	notifyFilterFunc NotifyFilterFunc
}

func (s *subscriber) PendingTransactionHash() <-chan string {
	return s.ch
}

func (s *subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *subscriber) Unsubscribe() {
	s.unsubscribe()
}

type NotifyFilterFunc func(pendingTxnHash string) bool
