package ethmonitor

type Subscription interface {
	Blocks() <-chan Blocks
	Done() <-chan struct{}
	Unsubscribe()
}

type subscriber struct {
	ch          chan Blocks
	done        chan struct{}
	unsubscribe func()
}

func (s *subscriber) Blocks() <-chan Blocks {
	return s.ch
}

func (s *subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *subscriber) Unsubscribe() {
	s.unsubscribe()
}
