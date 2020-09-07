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

type GasGauge struct {
	ctx     context.Context
	ctxStop context.CancelFunc
	started bool
	running sync.WaitGroup
	mu      sync.RWMutex

	logger                   util.Logger
	ethMonitor               *ethmonitor.Monitor
	suggestedGasPrice        SuggestedGasPrice
	suggestedGasPriceUpdated *sync.Cond
}

type SuggestedGasPrice struct {
	Rapid    uint64 `json:"rapid"` // in gwei
	Fast     uint64 `json:"fast"`
	Standard uint64 `json:"standard"`
	Slow     uint64 `json:"slow"`

	BlockNum  *big.Int `json:"blockNum"`
	BlockTime uint64   `json:"blockTime"`
}

func NewGasGauge(logger util.Logger, monitor *ethmonitor.Monitor) (*GasGauge, error) {
	return &GasGauge{
		logger:                   logger,
		ethMonitor:               monitor,
		suggestedGasPriceUpdated: sync.NewCond(&sync.Mutex{}),
	}, nil
}

func (g *GasGauge) Start(ctx context.Context) error {
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

func (g *GasGauge) Stop() error {
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

func (g *GasGauge) run() error {
	sub := g.ethMonitor.Subscribe()
	defer sub.Unsubscribe()

	ema1 := NewEMA(0.9)
	ema30 := NewEMA(0.5)
	ema70 := NewEMA(0.5)
	ema95 := NewEMA(0.5)

	// n1 := (60 * 10) / 15 // ~15 seconds per block
	// n30 := (60 * 2) / 15
	// n75 := (60 * 1) / 15

	// a1 := []uint64{}
	// a30 := []uint64{}
	// a75 := []uint64{}

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
			p1 := percentileValue(gasPrices, 0.01)
			p30 := percentileValue(gasPrices, 0.3)
			p70 := percentileValue(gasPrices, 0.7)
			p95 := percentileValue(gasPrices, 0.95)

			// add sample to cumulative exponentially moving average
			ema1.Tick(new(big.Int).SetUint64(p1))
			ema30.Tick(new(big.Int).SetUint64(p30))
			ema70.Tick(new(big.Int).SetUint64(p70))
			ema95.Tick(new(big.Int).SetUint64(p95))

			// compute final suggested gas price by averaging the samples
			// over a period of time
			sgp := SuggestedGasPrice{
				BlockNum:  latestBlock.Number(),
				BlockTime: latestBlock.Time(),
				// Fast:      periodEMA(ema75.Value().Uint64()/1e9, &a75, n75),
				// Standard:  periodEMA(ema30.Value().Uint64()/1e9, &a30, n30),
				// Slow:      periodEMA(ema1.Value().Uint64()/1e9, &a1, n1),
				Rapid:    ema95.Value().Uint64() / 1e9,
				Fast:     ema70.Value().Uint64() / 1e9,
				Standard: ema30.Value().Uint64() / 1e9,
				Slow:     ema1.Value().Uint64() / 1e9,
			}

			g.suggestedGasPriceUpdated.L.Lock()
			g.suggestedGasPrice = sgp
			g.suggestedGasPriceUpdated.Broadcast()
			g.suggestedGasPriceUpdated.L.Unlock()

		// eth monitor has stopped
		case <-sub.Done():
			return nil

		// gas tracker service is stopping
		case <-g.ctx.Done():
			return nil
		}
	}
}

func (g *GasGauge) SuggestedGasPrice() SuggestedGasPrice {
	return g.suggestedGasPrice
}

func (g *GasGauge) WaitSuggestedGasPrice() SuggestedGasPrice {
	g.suggestedGasPriceUpdated.L.Lock()
	g.suggestedGasPriceUpdated.Wait()
	v := g.suggestedGasPrice
	g.suggestedGasPriceUpdated.L.Unlock()
	return v
}

func (g *GasGauge) Subscribe() ethmonitor.Subscription {
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
	ema := NewEMA(0.2)
	for _, v := range *group {
		ema.Tick(new(big.Int).SetUint64(v))
	}
	return ema.Value().Uint64()
}
