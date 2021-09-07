package ethmonitor

import (
	"fmt"
	"sync"
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

type queue struct {
	events []Blocks
	cap    int
	mu     sync.Mutex
}

func newQueue(cap int) *queue {
	return &queue{
		events: make([]Blocks, 0, cap),
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
	// fmt.Println("enqueue!", len(events))
	c.events = append(c.events, events)
	if len(c.events) > c.cap {
		return fmt.Errorf("queue is full, it must be dequeued first..")
	}
	return nil
}

func (c *queue) dequeue(maxBlockNum uint64) []Blocks {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.events) == 0 {
		return []Blocks{} // queue is empty
	}

	var events []Blocks
	i := 0

	for _, ev := range c.events {
		if ev.IsOK() {
			// maxBlockNum indicates we want to "trail-behind", and only dequeue
			// up to a certain limit.
			if maxBlockNum > 0 && ev.LatestBlock().Block.NumberU64() > maxBlockNum {
				break
			}

			// collect dequeued events
			events = append(events, ev)
			i++
		} else {
			break
		}
	}

	// trim queue and return dequeued events
	c.events = c.events[i:]
	return events
}

func (c *queue) head() Blocks {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return nil
	}
	return c.events[0]
}

func (c *queue) tail() Blocks {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return nil
	}
	return c.events[len(c.events)-1]
}
