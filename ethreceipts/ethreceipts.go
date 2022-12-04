package ethreceipts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/goware/breaker"
	"github.com/goware/logger"
)

const (
	maxConcurrentFetchReceipts = 30
	// pastReceiptsBufSize        = 4000
)

type ReceiptListener struct {
	log      logger.Logger
	provider *ethrpc.Provider
	monitor  *ethmonitor.Monitor
	br       *breaker.Breaker

	receiptsSem chan struct{}

	// pastReceipts   []BlockOfReceipts
	// muPastReceipts sync.Mutex

	subscribers   []*subscriber
	muSubscribers sync.Mutex

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
	mu      sync.RWMutex
}

type subscriber struct {
	// ch          <-chan ReceiptResult
	// sendCh      chan<- ReceiptResult
	done        chan struct{}
	unsubscribe func()
}

var (
	ErrBlah = errors.New("ethreceipts: x")
)

func NewReceiptListener(log logger.Logger, provider *ethrpc.Provider, monitor *ethmonitor.Monitor) (*ReceiptListener, error) {
	if !monitor.Options().WithLogs {
		return nil, fmt.Errorf("ReceiptListener needs a monitor with WithLogs enabled to function")
	}

	return &ReceiptListener{
		log:         log,
		provider:    provider,
		monitor:     monitor,
		br:          breaker.New(log, 1*time.Second, 2, 10),
		receiptsSem: make(chan struct{}, maxConcurrentFetchReceipts),
		// pastReceipts: make([]BlockOfReceipts, 0),
		subscribers: make([]*subscriber, 0),
	}, nil
}

func (l *ReceiptListener) Run(ctx context.Context) error {
	if l.IsRunning() {
		return fmt.Errorf("ethmonitor: already running")
	}

	l.ctx, l.ctxStop = context.WithCancel(ctx)

	atomic.StoreInt32(&l.running, 1)
	defer atomic.StoreInt32(&l.running, 0)

	return l.listener()
}

func (l *ReceiptListener) Stop() {
	l.log.Info("ethreceipts: stop")
	l.ctxStop()
}

func (l *ReceiptListener) IsRunning() bool {
	return atomic.LoadInt32(&l.running) == 1
}

func (l *ReceiptListener) listener() error {
	sub := l.monitor.Subscribe()
	defer sub.Unsubscribe()

	for {
		select {

		case <-l.ctx.Done():
			l.log.Debug("ethreceipts: parent signaled to cancel - receipt listener is quitting")
			return nil

		case <-sub.Done():
			l.log.Info("ethreceipts: receipt listener is stopped because monitor signaled its stopping")
			return nil

		case blocks := <-sub.Blocks():
			fmt.Println("blocks", len(blocks))
			for _, block := range blocks {
				l.handleBlock(l.ctx, block)
			}
		}
	}
}

func (l *ReceiptListener) handleBlock(ctx context.Context, block *ethmonitor.Block) {
	// TODO ..
	// spew.Dump(block)
	fmt.Println("suppp")
}
