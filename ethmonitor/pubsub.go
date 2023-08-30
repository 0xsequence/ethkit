package ethmonitor

import (
	"fmt"
	"sync"

	"github.com/goware/channel"
	"github.com/goware/superr"
)

type Subscription interface {
	Blocks() <-chan Blocks
	Done() <-chan struct{}
	Err() error
	Unsubscribe()
}

var _ Subscription = &subscriber{}

type subscriber struct {
	ch              channel.Channel[Blocks]
	done            chan struct{}
	err             error
	unsubscribe     func()
	unsubscribeOnce sync.Once
}

func (s *subscriber) Blocks() <-chan Blocks {
	return s.ch.ReadChannel()
}

func (s *subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *subscriber) Err() error {
	return s.err
}

func (s *subscriber) Unsubscribe() {
	s.unsubscribeOnce.Do(s.unsubscribe)
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

	for _, event := range events {
		switch event.Event {

		case Added:
			c.events = append(c.events, event)

		case Removed:
			if len(c.events) > 0 {
				tail := c.events[len(c.events)-1]

				switch tail.Event {
				case Added:
					if event.Hash() == tail.Hash() {
						// instead of publishing this removal, pop the most recent event
						c.events = c.events[:len(c.events)-1]
					} else {
						// it should be impossible to remove anything but the most recent event
						return fmt.Errorf("removing block %v %v %v, but last block is %v %v %v", event.Event, event.Number(), event.Hash().Hex(), tail.Event, tail.Number(), tail.Hash().Hex())
					}
				case Removed:
					// we have a string of removal events, so we can only publish the removal
					c.events = append(c.events, event)
				}
			} else {
				// we already published the addition, so we must publish the removal
				c.events = append(c.events, event)
			}

		default:
			return fmt.Errorf("unknown event type %v %v %v", event.Event, event.Number(), event.Hash().Hex())
		}
	}

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

	for _, ev := range c.events {
		if ev.OK {
			// maxBlockNum indicates we want to "trail-behind", and only dequeue
			// up to a certain limit.
			if maxBlockNum > 0 && ev.Block.NumberU64() > maxBlockNum {
				break
			}

			// collect dequeued events
			events = append(events, ev)
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
	c.events = c.events[len(events):]

	return events, true
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
