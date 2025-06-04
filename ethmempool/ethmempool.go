package ethmempool

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/0xsequence/ethkit/go-ethereum/rpc"
)

type Options struct {
	Logger *slog.Logger
}

type Mempool struct {
	options Options

	log              *slog.Logger
	nodeWebsocketURL string
	client           *rpc.Client
	subscribers      []*subscriber

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
	mu      sync.RWMutex
}

func NewMempool(nodeWebsocketURL string, opts ...Options) (*Mempool, error) {
	options := Options{}
	if len(opts) > 0 {
		options = opts[0]
	}

	return &Mempool{
		options:          options,
		nodeWebsocketURL: nodeWebsocketURL,
		subscribers:      make([]*subscriber, 0),
	}, nil
}

func (m *Mempool) Run(ctx context.Context) error {
	if m.IsRunning() {
		return fmt.Errorf("ethmempool: already running")
	}

	m.ctx, m.ctxStop = context.WithCancel(ctx)

	atomic.StoreInt32(&m.running, 1)
	defer atomic.StoreInt32(&m.running, 0)

	// open websocket connection
	var err error
	m.client, err = rpc.Dial(m.nodeWebsocketURL)
	if err != nil {
		return fmt.Errorf("ethmempool: failed to open websocket connection to %s: %w", m.nodeWebsocketURL, err)
	}

	// stream events and broadcast to subscribers
	return m.stream()
}

func (m *Mempool) Stop() {
	m.ctxStop()

	// disconnect websocket
	m.client.Close()
}

func (m *Mempool) IsRunning() bool {
	return atomic.LoadInt32(&m.running) == 1
}

func (m *Mempool) Options() Options {
	return m.options
}

func (m *Mempool) Subscribe() Subscription {
	return m.subscribe(nil)
}

func (m *Mempool) SubscribeWithFilter(notifyFilterFunc NotifyFilterFunc) Subscription {
	return m.subscribe(notifyFilterFunc)
}

func (m *Mempool) subscribe(notifyFilterFunc NotifyFilterFunc) Subscription {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscriber := &subscriber{
		ch:               make(chan string, 1024),
		done:             make(chan struct{}),
		notifyFilterFunc: notifyFilterFunc, // optional, can be nil
	}

	subscriber.unsubscribe = func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, sub := range m.subscribers {
			if sub == subscriber {
				m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
				close(subscriber.done)
				close(subscriber.ch)
				return
			}
		}
	}

	m.subscribers = append(m.subscribers, subscriber)

	return subscriber
}

func (m *Mempool) stream() error {
	ch := make(chan string) // txn hash strings

	sub, err := m.client.EthSubscribe(m.ctx, ch, "newPendingTransactions")
	if err != nil {
		return fmt.Errorf("ethmempool: stream failed to subscribe %w", err)
	}
	defer sub.Unsubscribe()

	for {
		select {

		case <-m.ctx.Done():
			return nil

		case <-sub.Err():
			return fmt.Errorf("ethmempool: websocket error %w", err)

		case pendingTxnHash := <-ch:
			// notify all subscribers..
			pendingTxnHash = strings.ToLower(pendingTxnHash)
			for _, sub := range m.subscribers {
				if sub.notifyFilterFunc == nil || sub.notifyFilterFunc(pendingTxnHash) {
					sub.ch <- pendingTxnHash
				}
			}
		}
	}
}
