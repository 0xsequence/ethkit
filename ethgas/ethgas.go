package ethgas

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/util"
)

const (
	ONE_GWEI = uint64(1e9)
)

type GasGauge struct {
	log                      util.Logger
	ethMonitor               *ethmonitor.Monitor
	suggestedGasPrice        SuggestedGasPrice
	suggestedGasPriceUpdated *sync.Cond
	useEIP1559               bool // TODO: currently not in use, but once we think about block utilization, then will be useful
	minGasPriceInGwei        uint64

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
}

type SuggestedGasPrice struct {
	Instant  uint64 `json:"instant"` // in gwei
	Fast     uint64 `json:"fast"`
	Standard uint64 `json:"standard"`
	Slow     uint64 `json:"slow"`

	BlockNum  *big.Int `json:"blockNum"`
	BlockTime uint64   `json:"blockTime"`
}

func NewGasGauge(log util.Logger, monitor *ethmonitor.Monitor, minGasPriceInGwei uint64, useEIP1559 bool) (*GasGauge, error) {
	if minGasPriceInGwei > ONE_GWEI {
		return nil, fmt.Errorf("minGasPriceInGwei argument expected to be passed as Gwei, but your units look like wei")
	}
	if minGasPriceInGwei == 0 {
		return nil, fmt.Errorf("minGasPriceInGwei cannot be 0, pass at least 1")
	}
	return &GasGauge{
		log:                      log,
		ethMonitor:               monitor,
		minGasPriceInGwei:        minGasPriceInGwei,
		useEIP1559:               useEIP1559,
		suggestedGasPriceUpdated: sync.NewCond(&sync.Mutex{}),
	}, nil
}

func (g *GasGauge) Run(ctx context.Context) error {
	if g.IsRunning() {
		return fmt.Errorf("ethgas: already running")
	}

	g.ctx, g.ctxStop = context.WithCancel(ctx)

	atomic.StoreInt32(&g.running, 1)
	defer atomic.StoreInt32(&g.running, 0)

	return g.run()
}

func (g *GasGauge) Stop() {
	g.ctxStop()
}

func (g *GasGauge) IsRunning() bool {
	return atomic.LoadInt32(&g.running) == 1
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

func (g *GasGauge) run() error {
	sub := g.ethMonitor.Subscribe()
	defer sub.Unsubscribe()

	var instant, fast, standard, slow uint64 = 0, 0, 0, 0

	minGasPriceInWei := g.minGasPriceInGwei * ONE_GWEI

	emaSlow := NewEMA(0.5)
	emaStandard := NewEMA(0.5)
	emaFast := NewEMA(0.5)
	emaInstant := NewEMA(0.5)

	for {
		select {

		// service is stopping
		case <-g.ctx.Done():
			return nil

		// eth monitor has stopped
		case <-sub.Done():
			return fmt.Errorf("ethmonitor has stopped so the gauge cannot continue, stopping.")

		// received new mined block from ethmonitor
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
				var gasPrice uint64

				switch txn.Type() {
				case types.LegacyTxType:
					gasPrice = txn.GasPrice().Uint64()
				case types.AccessListTxType:
					gasPrice = txn.GasPrice().Uint64()
				case types.DynamicFeeTxType:
					gasPrice = txn.GasTipCap().Uint64() + latestBlock.BaseFee().Uint64()
				}

				if gasPrice < minGasPriceInWei {
					continue // skip prices which are outliers / "deals with miner"
				}
				gasPrices = append(gasPrices, gasPrice)
			}

			// Case if there are no gas prices sampled from any of the transactions, lets
			// query the node for a price, or use our minimum (whichever is higher).
			if len(gasPrices) == 0 {
				networkSuggestedPrice := minGasPriceInWei
				ethGasPrice, _ := g.ethMonitor.Provider().SuggestGasPrice(context.Background())
				if ethGasPrice == nil || ethGasPrice.Uint64() > minGasPriceInWei {
					networkSuggestedPrice = ethGasPrice.Uint64()
				}
				gasPrices = append(gasPrices, networkSuggestedPrice)
			}

			// sort gas list from low to high
			sort.Slice(gasPrices, func(i, j int) bool {
				return gasPrices[i] < gasPrices[j]
			})

			// get gas price from list at percentile position
			p40 := percentileValue(gasPrices, 0.4)  // low
			p75 := percentileValue(gasPrices, 0.75) // mid
			p90 := percentileValue(gasPrices, 0.9)  // expensive

			// TODO: lets consider the block GasLimit, GasUsed, and multipler of the node
			// so we can account for the utilization of a block on the network and consider it as a factor of the gas price

			instant = uint64(math.Max(float64(p90), float64(minGasPriceInWei)))
			fast = uint64(math.Max(float64(p75), float64(minGasPriceInWei)))
			standard = uint64(math.Max(float64(p40), float64(minGasPriceInWei)))
			slow = uint64(math.Max(float64(standard)*0.85, float64(minGasPriceInWei)))

			// tick
			emaSlow.Tick(new(big.Int).SetUint64(slow))
			emaStandard.Tick(new(big.Int).SetUint64(standard))
			emaFast.Tick(new(big.Int).SetUint64(fast))
			emaInstant.Tick(new(big.Int).SetUint64(instant))

			// compute final suggested gas price by averaging the samples
			// over a period of time
			sgp := SuggestedGasPrice{
				BlockNum:  latestBlock.Number(),
				BlockTime: latestBlock.Time(),
				Instant:   uint64(emaInstant.Value().Uint64() / 1e9),
				Fast:      uint64(emaFast.Value().Uint64() / 1e9),
				Standard:  uint64(emaStandard.Value().Uint64() / 1e9),
				Slow:      uint64(emaSlow.Value().Uint64() / 1e9),
			}

			g.suggestedGasPriceUpdated.L.Lock()
			g.suggestedGasPrice = sgp
			g.suggestedGasPriceUpdated.Broadcast()
			g.suggestedGasPriceUpdated.L.Unlock()

		}
	}
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
