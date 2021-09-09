package ethmonitor

import (
	"fmt"
	"sync"

	"github.com/goware/superr"
)

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

// queue is the publish event queue
type queue struct {
	events Blocks
	cap    int
	mu     sync.Mutex
}

func newQueue(cap int) *queue {
	return &queue{
		events: make(Blocks, 0, cap),
		cap:    cap,
	}
}

func (q *queue) clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = q.events[:0]
}

func (q *queue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.events)
}

func (c *queue) enqueue(events Blocks) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Println("enqueue!", len(events))
	c.events = append(c.events, events...)
	if len(c.events) > c.cap {
		return superr.New(ErrFatal, ErrQueueFull)
	}
	return nil
}

func (c *queue) dequeue(maxBlockNum uint64) (Blocks, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.events) == 0 {
		return Blocks{}, false // queue is empty
	}

	var events Blocks
	i := 0

	for _, ev := range c.events {
		if ev.OK {
			// maxBlockNum indicates we want to "trail-behind", and only dequeue
			// up to a certain limit.
			if maxBlockNum > 0 && ev.Block.NumberU64() > maxBlockNum {
				break
			}

			// collect dequeued events
			events = append(events, ev)
			i++
		} else {
			break
		}
	}

	if len(events) == 0 {
		return Blocks{}, false
	}
	if events[len(events)-1].Event != Added {
		// last block must be an added one, otherwise we do
		// not dequeue any events
		return Blocks{}, false
	}

	// trim queue and return dequeued events
	c.events = c.sweep(c.events[i:])

	return events, true
}

func (c *queue) sweep(events Blocks) Blocks {
	// TODO: we can sweep remaining published events to remove reorg de-dupe
	// and clean the history while in trail-behind mode to "join" reorgs etc.
	//
	// NOTE: small edge case, where.. we could "publish" a block which we don't have logs for.. which would enqueue it
	// but not send it ..
	// then, turns out, we need to revert it.. and previous value was also updated.. we can de-dupe, but in this case
	// the removal is of a block with zero logs, so its pretty much a noop.
	return events
}

func (c *queue) head() *Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return nil
	}
	return c.events[0]
}

func (c *queue) tail() *Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return nil
	}
	return c.events[len(c.events)-1]
}
