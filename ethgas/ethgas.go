package ethgas

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/util"
)

const MIN_GAS_PRICE = uint64(1e9)

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
	Instant uint64 `json:"instant"` // in gwei
	Fast    uint64 `json:"fast"`
	Normal  uint64 `json:"normal"`
	Slow    uint64 `json:"slow"`

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

	var instant, fast, normal, slow uint64 = 0, 0, 0, 0

	ema1 := NewEMA(0.5)
	ema30 := NewEMA(0.5)
	ema70 := NewEMA(0.5)
	ema95 := NewEMA(0.5)

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
				if gp <= MIN_GAS_PRICE {
					continue // skip prices which are outliers / "deals with miner"
				}
				gasPrices = append(gasPrices, txn.GasPrice().Uint64())
			}

			var networkSuggestedPrice uint64 = 0
			ethGasPrice, _ := g.ethMonitor.Provider().SuggestGasPrice(context.Background())
			if ethGasPrice == nil {
				networkSuggestedPrice = MIN_GAS_PRICE
			} else {
				networkSuggestedPrice = ethGasPrice.Uint64()
			}

			if len(gasPrices) == 0 {
				gasPrices = append(gasPrices, networkSuggestedPrice)
			}

			// sort gas list from low to high
			sort.Slice(gasPrices, func(i, j int) bool {
				return gasPrices[i] < gasPrices[j]
			})

			// get gas price from list at percentile position
			p30 := percentileValue(gasPrices, 0.3)  // low
			p70 := percentileValue(gasPrices, 0.7)  // mid
			p95 := percentileValue(gasPrices, 0.95) // expensive

			// block gas utilization
			blockUtil := float64(latestBlock.GasUsed()) / float64(latestBlock.GasLimit())

			// calculate taking unused gas into account
			gasUnused := latestBlock.GasLimit() - latestBlock.GasUsed()
			avgTxSize := latestBlock.GasUsed() / uint64(len(txns))

			if gasUnused >= avgTxSize {
				instant = uint64(math.Max(float64(p95)*blockUtil, float64(networkSuggestedPrice)))
				fast = uint64(math.Max(float64(p70)*blockUtil, float64(networkSuggestedPrice)))
				normal = uint64(math.Max(float64(p30)*blockUtil, float64(networkSuggestedPrice)))
				slow = uint64(networkSuggestedPrice)
			} else {
				instant = p95
				fast = p70
				normal = p30
				slow = uint64(float64(normal) * 0.85)
			}

			// tick
			ema1.Tick(new(big.Int).SetUint64(slow))
			ema30.Tick(new(big.Int).SetUint64(normal))
			ema70.Tick(new(big.Int).SetUint64(fast))
			ema95.Tick(new(big.Int).SetUint64(instant))

			// compute final suggested gas price by averaging the samples
			// over a period of time
			sgp := SuggestedGasPrice{
				BlockNum:  latestBlock.Number(),
				BlockTime: latestBlock.Time(),
				Instant:   uint64(ema95.Value().Uint64() / 1e9),
				Fast:      uint64(ema70.Value().Uint64() / 1e9),
				Normal:    uint64(ema30.Value().Uint64() / 1e9),
				Slow:      uint64(ema1.Value().Uint64() / 1e9),
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
