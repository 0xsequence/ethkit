package ethgas

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/arcadeum/ethkit/ethmonitor"
	"github.com/arcadeum/ethkit/util"
)

type GasTracker struct {
	ctx     context.Context
	ctxStop context.CancelFunc
	started bool
	running sync.WaitGroup
	mu      sync.RWMutex

	logger            util.Logger
	ethMonitor        *ethmonitor.Monitor
	suggestedGasPrice SuggestedGasPrice
}

type SuggestedGasPrice struct {
	Fast   uint64 `json:"fast"` // in gwei
	Normal uint64 `json:"normal"`
	Slow   uint64 `json:"slow"`

	BlockNum  *big.Int `json:"blockNum"`
	BlockTime uint64   `json:"blockTime"`
}

func NewGasTracker(logger util.Logger, monitor *ethmonitor.Monitor) (*GasTracker, error) {
	return &GasTracker{
		logger:     logger,
		ethMonitor: monitor,
	}, nil
}

func (g *GasTracker) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started {
		return fmt.Errorf("already started")
	}
	g.started = true

	g.ctx, g.ctxStop = context.WithCancel(ctx)

	go func() {
		g.running.Add(1)
		defer g.running.Done()
		g.run()
	}()

	return nil
}

func (g *GasTracker) Stop() error {
	g.mu.Lock()
	if !g.started {
		g.mu.Unlock()
		return nil
	}

	g.started = false
	g.ctxStop()
	g.mu.Unlock()

	g.running.Wait()
	return nil
}

func (g *GasTracker) run() error {
	sub := g.ethMonitor.Subscribe()
	defer sub.Unsubscribe()

	ema20 := NewEMA(0.2)
	ema50 := NewEMA(0.2)
	ema75 := NewEMA(0.2)

	n20 := (60 * 10) / 15 // ~15 seconds per block
	n50 := (60 * 3) / 15
	n75 := (60 * 1) / 15

	a20 := []uint64{}
	a50 := []uint64{}
	a75 := []uint64{}

	for {
		select {

		case blocks := <-sub.Blocks():
			latestBlock := blocks.LatestBlock()
			if latestBlock == nil {
				continue
			}

			txns := latestBlock.Transactions()
			if len(txns) == 0 {
				continue
			}

			gasPrices := []uint64{}
			for _, txn := range txns {
				gp := txn.GasPrice().Uint64()
				if gp <= 1e9 {
					continue // skip prices which are outliers / "deals with miner"
				}
				gasPrices = append(gasPrices, txn.GasPrice().Uint64())
			}
			if len(gasPrices) == 0 {
				continue
			}

			// sort gas list from low to high
			sort.Slice(gasPrices, func(i, j int) bool {
				return gasPrices[i] < gasPrices[j]
			})

			// get gas price from list at percentile position
			p20 := percentileValue(gasPrices, 0.20)
			p50 := percentileValue(gasPrices, 0.50)
			p75 := percentileValue(gasPrices, 0.75)

			// add sample to cumulative exponentially moving average
			ema20.Tick(new(big.Int).SetUint64(p20))
			ema50.Tick(new(big.Int).SetUint64(p50))
			ema75.Tick(new(big.Int).SetUint64(p75))

			// compute final suggested gas price by averaging the samples
			// over a period of time
			sgp := SuggestedGasPrice{
				BlockNum:  latestBlock.Number(),
				BlockTime: latestBlock.Time(),
				Fast:      periodEMA(ema75.Value().Uint64()/1e9, &a75, n75),
				Normal:    periodEMA(ema50.Value().Uint64()/1e9, &a50, n50),
				Slow:      periodEMA(ema20.Value().Uint64()/1e9, &a20, n20),
			}
			g.suggestedGasPrice = sgp

		// eth monitor has stopped
		case <-sub.Done():
			return nil

		// gas tracker service is stopping
		case <-g.ctx.Done():
			return nil
		}
	}
}

func (g *GasTracker) SuggestedGasPrice() SuggestedGasPrice {
	return g.suggestedGasPrice
}

func (g *GasTracker) Subscribe() ethmonitor.Subscription {
	return g.ethMonitor.Subscribe()
}

func percentileValue(list []uint64, percentile float64) uint64 {
	return list[int(float64(len(list)-1)*percentile)]
}

func periodEMA(price uint64, group *[]uint64, size int) uint64 {
	*group = append(*group, price)
	if len(*group) > size {
		*group = (*group)[1:]
	}
	ema := NewEMA(0.05) // 5% decay
	for _, v := range *group {
		ema.Tick(new(big.Int).SetUint64(v))
	}
	return ema.Value().Uint64()
}
