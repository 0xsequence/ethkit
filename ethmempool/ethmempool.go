package ethmempool

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/0xsequence/ethkit/go-ethereum/rpc"
	"github.com/0xsequence/ethkit/util"
)

type Options struct {
	Logger util.Logger
}

type Mempool struct {
	options Options

	ctx     context.Context
	ctxStop context.CancelFunc

	log              util.Logger
	nodeWebsocketURL string
	client           *rpc.Client
	subscribers      []*subscriber

	started bool
	running sync.WaitGroup
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

func (m *Mempool) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("ethmempool: already started")
	}
	m.started = true

	// open websocket connection
	var err error
	m.client, err = rpc.Dial(m.nodeWebsocketURL)
	if err != nil {
		return fmt.Errorf("ethmempool: failed to open websocket connection to %s: %w", m.nodeWebsocketURL, err)
	}

	// stream events and broadcast to subscribers
	m.ctx, m.ctxStop = context.WithCancel(ctx)
	go func() {
		m.running.Add(1)
		defer m.running.Done()
		m.stream(m.ctx)
	}()

	return nil
}

func (m *Mempool) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}

	m.started = false
	m.ctxStop()
	m.mu.Unlock()

	// disconnect websocket
	m.client.Close()

	m.running.Wait()
	return nil
}

func (m *Mempool) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
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

func (m *Mempool) stream(ctx context.Context) error {
	ch := make(chan string) // txn hash strings

	sub, err := m.client.EthSubscribe(ctx, ch, "newPendingTransactions")
	if err != nil {
		return fmt.Errorf("ethmempool: stream failed to subscribe %w", err)
	}
	defer sub.Unsubscribe()

	for {
		select {

		case <-ctx.Done():
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
