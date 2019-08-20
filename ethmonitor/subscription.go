package ethmonitor

type EventType uint32

const (
	Added = iota
	Removed
)

type Event struct {
	Type  EventType
	Block *Block
}

type Events []Event

func (e Events) LatestBlock() *Block {
	for i := len(e) - 1; i >= 0; i-- {
		if e[i].Type == Added {
			return e[i].Block
		}
	}
	return nil
}

type Subscription interface {
	Events() <-chan Events
	Done() <-chan struct{}
	Unsubscribe() func()
}

type subscriber struct {
	ch          chan Events
	done        chan struct{}
	unsubscribe func()
}

func (s *subscriber) Events() <-chan Events {
	return s.ch
}

func (s *subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *subscriber) Unsubscribe() func() {
	return s.unsubscribe
}
