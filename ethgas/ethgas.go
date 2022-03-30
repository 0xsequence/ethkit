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
	"github.com/0xsequence/ethkit/util"
)

const (
	MIN_GAS_PRICE = uint64(1e9)
	ONE_GWEI      = uint64(1e9)
)

type GasGauge struct {
	log                      util.Logger
	ethMonitor               *ethmonitor.Monitor
	suggestedGasPrice        SuggestedGasPrice
	suggestedGasPriceUpdated *sync.Cond

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

func NewGasGauge(log util.Logger, monitor *ethmonitor.Monitor) (*GasGauge, error) {
	return &GasGauge{
		log:                      log,
		ethMonitor:               monitor,
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

	ema1 := NewEMA(0.5)
	ema30 := NewEMA(0.5)
	ema70 := NewEMA(0.5)
	ema95 := NewEMA(0.5)

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

			fmt.Println("new block..", latestBlock.Block.NumberU64())

			txns := latestBlock.Transactions()
			if len(txns) == 0 {
				continue
			}

			gasPrices := []uint64{}
			for _, txn := range txns {
				var gasPrice uint64

				if txn.Type() == 0 {
					gasPrice = txn.GasPrice().Uint64()
				} else {
					// fmt.Println("zzzz", txn.GasPrice().Uint64(), txn.GasTipCap().Uint64(), txn.GasFeeCap().Uint64())
					// gasPrices = append(gasPrices, txn.GasPrice().Uint64()+txn.GasFeeCap().Uint64())
					gasPrice = txn.GasTipCap().Uint64() + latestBlock.BaseFee().Uint64()
				}

				if gasPrice <= MIN_GAS_PRICE {
					continue // skip prices which are outliers / "deals with miner"
				}
				gasPrices = append(gasPrices, gasPrice)
			}

			var networkSuggestedPrice uint64 = 0
			ethGasPrice, _ := g.ethMonitor.Provider().SuggestGasPrice(context.Background())
			if ethGasPrice == nil {
				fmt.Println("ethGasPrice is nil... okeeee...")
				networkSuggestedPrice = MIN_GAS_PRICE
			} else {
				fmt.Println("network suggested price is......", ethGasPrice.Uint64())
				networkSuggestedPrice = ethGasPrice.Uint64()
			}

			if len(gasPrices) == 0 {
				gasPrices = append(gasPrices, networkSuggestedPrice)
			}

			// sort gas list from low to high
			sort.Slice(gasPrices, func(i, j int) bool {
				return gasPrices[i] < gasPrices[j]
			})

			// spew.Dump(gasPrices)

			// get gas price from list at percentile position
			p30 := percentileValue(gasPrices, 0.3) // low
			p80 := percentileValue(gasPrices, 0.8) // mid
			p90 := percentileValue(gasPrices, 0.9) // expensive

			// block gas utilization
			multiplier := uint64(2)
			blockUtil := float64(latestBlock.GasUsed()) / float64(latestBlock.GasLimit()/multiplier)

			fmt.Println("==>  gasUsed", latestBlock.GasUsed())
			fmt.Println("==> gasLimit", latestBlock.GasLimit())
			// fmt.Println("==> baseFee", latestBlock.BaseFee()) // in Gwei
			// fmt.Println("==>  baseFee", new(big.Int).Mul(latestBlock.BaseFee(), new(big.Int).SetUint64(ONE_GWEI)))

			// calculate taking unused gas into account
			gasUnused := latestBlock.GasLimit() - latestBlock.GasUsed()
			avgTxSize := latestBlock.GasUsed() / uint64(len(txns))

			// networkSuggestedPrice += new(big.Int).Mul(latestBlock.BaseFee(), new(big.Int).SetUint64(ONE_GWEI)).Uint64()

			fmt.Println("==> suggested price..", networkSuggestedPrice)

			fmt.Println("gas prices.. 0.80:", p80)
			fmt.Println(".. our calc..., blockUtil", blockUtil)

			if gasUnused >= avgTxSize {
				instant = uint64(math.Max(float64(p90)*blockUtil, float64(networkSuggestedPrice)))
				fast = uint64(math.Max(float64(p80)*blockUtil, float64(networkSuggestedPrice)))
				standard = uint64(math.Max(float64(p30)*blockUtil, float64(networkSuggestedPrice)))
				slow = uint64(networkSuggestedPrice)
			} else {
				instant = p90
				fast = p80
				standard = p30
				slow = uint64(float64(standard) * 0.85)
			}

			// tick
			ema1.Tick(new(big.Int).SetUint64(slow))
			ema30.Tick(new(big.Int).SetUint64(standard))
			ema70.Tick(new(big.Int).SetUint64(fast))
			ema95.Tick(new(big.Int).SetUint64(instant))

			// compute final suggested gas price by averaging the samples
			// over a period of time
			sgp := SuggestedGasPrice{
				BlockNum:  latestBlock.Number(),
				BlockTime: latestBlock.Time(),
				Instant:   uint64(ema95.Value().Uint64() / 1e9),
				Fast:      uint64(ema70.Value().Uint64() / 1e9),
				Standard:  uint64(ema30.Value().Uint64() / 1e9),
				Slow:      uint64(ema1.Value().Uint64() / 1e9),
			}

			fmt.Println("===> instant/fast", sgp.Instant, sgp.Fast)

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
