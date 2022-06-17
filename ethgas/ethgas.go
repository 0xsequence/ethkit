package ethgas

import (
	"context"
	"fmt"
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

var ONE_GWEI_BIG = big.NewInt(int64(ONE_GWEI))
var BUCKET_RANGE = big.NewInt(int64(5 * ONE_GWEI))

type GasGauge struct {
	log                      util.Logger
	ethMonitor               *ethmonitor.Monitor
	suggestedGasPrice        SuggestedGasPrice
	suggestedGasPriceUpdated *sync.Cond
	useEIP1559               bool // TODO: currently not in use, but once we think about block utilization, then will be useful
	minGasPrice              *big.Int

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
}

type SuggestedGasPrice struct {
	Instant  uint64 `json:"instant"` // in gwei
	Fast     uint64 `json:"fast"`
	Standard uint64 `json:"standard"`
	Slow     uint64 `json:"slow"`

	InstantWei  *big.Int `json:"instantWei"`
	FastWei     *big.Int `json:"fastWei"`
	StandardWei *big.Int `json:"standardWei"`
	SlowWei     *big.Int `json:"slowWei"`

	BlockNum  *big.Int `json:"blockNum"`
	BlockTime uint64   `json:"blockTime"`
}

func NewGasGaugeWei(log util.Logger, monitor *ethmonitor.Monitor, minGasPriceInWei uint64, useEIP1559 bool) (*GasGauge, error) {
	if minGasPriceInWei == 0 {
		return nil, fmt.Errorf("minGasPriceInGwei cannot be 0, pass at least 1")
	}
	return &GasGauge{
		log:                      log,
		ethMonitor:               monitor,
		minGasPrice:              big.NewInt(int64(minGasPriceInWei)),
		useEIP1559:               useEIP1559,
		suggestedGasPriceUpdated: sync.NewCond(&sync.Mutex{}),
	}, nil
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
		minGasPrice:              new(big.Int).Mul(big.NewInt(int64(minGasPriceInGwei)), ONE_GWEI_BIG),
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

			var gasPrices []*big.Int
			for _, txn := range txns {
				var gasPrice *big.Int

				switch txn.Type() {
				case types.LegacyTxType:
					gasPrice = txn.GasPrice()
				case types.AccessListTxType:
					gasPrice = txn.GasPrice()
				case types.DynamicFeeTxType:
					gasPrice = new(big.Int).Add(txn.GasTipCap(), latestBlock.BaseFee())
				}

				if gasPrice.Cmp(g.minGasPrice) < 0 {
					continue // skip prices which are outliers / "deals with miner"
				}
				gasPrices = append(gasPrices, gasPrice)
			}

			// Case if there are no gas prices sampled from any of the transactions, lets
			// query the node for a price, or use our minimum (whichever is higher).
			if len(gasPrices) == 0 {
				networkSuggestedPrice := new(big.Int).Set(g.minGasPrice)
				ethGasPrice, _ := g.ethMonitor.Provider().SuggestGasPrice(context.Background())
				if ethGasPrice != nil && ethGasPrice.Cmp(networkSuggestedPrice) > 0 {
					networkSuggestedPrice.Set(ethGasPrice)
				}
				gasPrices = append(gasPrices, networkSuggestedPrice)
			}

			// sort gas list from low to high
			sort.Slice(gasPrices, func(i, j int) bool {
				return gasPrices[i].Cmp(gasPrices[j]) < 0
			})

			// calculate gas price samples via histogram method
			hist := gasPriceHistogram(gasPrices)
			high, mid, low := hist.samplePrices()

			if high.Sign() == 0 || mid.Sign() == 0 || low.Sign() == 0 {
				continue
			}

			// get gas price from list at percentile position (old method)
			// high = percentileValue(gasPrices, 0.9) / ONE_GWEI  // expensive
			// mid = percentileValue(gasPrices, 0.75) / ONE_GWEI // mid
			// low = percentileValue(gasPrices, 0.4) / ONE_GWEI  // low

			// TODO: lets consider the block GasLimit, GasUsed, and multipler of the node
			// so we can account for the utilization of a block on the network and consider it as a factor of the gas price

			instant := max(high, g.minGasPrice)
			fast := max(mid, g.minGasPrice)
			standard := max(low, g.minGasPrice)
			slow := max(new(big.Int).Div(new(big.Int).Mul(standard, big.NewInt(85)), big.NewInt(100)), g.minGasPrice)

			// tick
			emaSlow.Tick(slow)
			emaStandard.Tick(standard)
			emaFast.Tick(fast)
			emaInstant.Tick(instant)

			// compute final suggested gas price by averaging the samples
			// over a period of time
			sgp := SuggestedGasPrice{
				BlockNum:    latestBlock.Number(),
				BlockTime:   latestBlock.Time(),
				InstantWei:  new(big.Int).Set(emaInstant.Value()),
				FastWei:     new(big.Int).Set(emaFast.Value()),
				StandardWei: new(big.Int).Set(emaStandard.Value()),
				SlowWei:     new(big.Int).Set(emaSlow.Value()),
				Instant:     new(big.Int).Div(emaInstant.Value(), ONE_GWEI_BIG).Uint64(),
				Fast:        new(big.Int).Div(emaFast.Value(), ONE_GWEI_BIG).Uint64(),
				Standard:    new(big.Int).Div(emaStandard.Value(), ONE_GWEI_BIG).Uint64(),
				Slow:        new(big.Int).Div(emaSlow.Value(), ONE_GWEI_BIG).Uint64(),
			}

			g.suggestedGasPriceUpdated.L.Lock()
			g.suggestedGasPrice = sgp
			g.suggestedGasPriceUpdated.Broadcast()
			g.suggestedGasPriceUpdated.L.Unlock()
		}
	}
}

func gasPriceHistogram(list []*big.Int) histogram {
	if len(list) == 0 {
		return histogram{}
	}

	min := new(big.Int).Set(list[0])
	hist := histogram{}

	b1 := new(big.Int).Set(min)
	b2 := new(big.Int).Add(min, BUCKET_RANGE)
	h := uint64(0)
	x := 0

	for _, v := range list {
		gp := new(big.Int).Set(v)

	fit:
		if gp.Cmp(b1) >= 0 && gp.Cmp(b2) < 0 {
			x++
			if h == 0 {
				h++
				hist = append(hist, histogramBucket{value: new(big.Int).Set(b1), count: 1})
			} else {
				h++
				hist[len(hist)-1].count = h
			}
		} else {
			h = 0
			b1.Add(b1, BUCKET_RANGE)
			b2.Add(b2, BUCKET_RANGE)
			goto fit
		}
	}

	// trim over-paying outliers
	hist2 := hist.trimOutliers()
	sort.Slice(hist2, hist2.sortByValue)

	return hist2
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

type histogram []histogramBucket

type histogramBucket struct {
	value *big.Int
	count uint64
}

func (h histogram) sortByCount(i, j int) bool {
	if h[i].count > h[j].count {
		return true
	}
	return h[i].count == h[j].count && h[i].value.Cmp(h[j].value) < 0
}

func (h histogram) sortByValue(i, j int) bool {
	return h[i].value.Cmp(h[j].value) < 0
}

func (h histogram) trimOutliers() histogram {
	h2 := histogram{}
	for _, v := range h {
		h2 = append(h2, v)
	}
	sort.Slice(h2, h2.sortByValue)

	if len(h2) == 0 {
		return h2
	}

	// for the last 25% of buckets, if we see a jump by 200%, then full-stop there
	x := int(float64(len(h2)) * 0.75)
	if x == len(h2) || x == 0 {
		return h2
	}

	h3 := h2[:x]
	last := h2[x-1].value
	for i := x; i < len(h2); i++ {
		v := h2[i].value
		if v.Cmp(new(big.Int).Mul(big.NewInt(2), last)) >= 0 {
			break
		}
		h3 = append(h3, h2[i])
		last = v
	}

	return h3
}

func (h histogram) percentileValue(percentile float64) *big.Int {
	if percentile < 0 {
		percentile = 0
	} else if percentile > 1 {
		percentile = 1
	}

	numSamples := uint64(0)

	for _, bucket := range h {
		numSamples += bucket.count
	}

	// suppose numSamples = 100
	// suppose percentile = 0.8
	// then we want to find the 80th sample and return its value

	// if percentile = 80%, then i want index = numSamples * 80%
	index := uint64(float64(numSamples) * percentile)
	// index = numSamples - 1

	// find the sample at index, then return its value
	numberOfSamplesConsidered := uint64(0)
	for _, bucket := range h {
		if numberOfSamplesConsidered+bucket.count > index {
			return new(big.Int).Set(bucket.value)
		}

		numberOfSamplesConsidered += bucket.count
	}

	return new(big.Int).Set(h[len(h)-1].value)
}

// returns sample inputs for: instant, fast, standard
func (h histogram) samplePrices() (*big.Int, *big.Int, *big.Int) {
	if len(h) == 0 {
		return big.NewInt(0), big.NewInt(0), big.NewInt(0)
	}

	sort.Slice(h, h.sortByValue)

	high := h.percentileValue(0.7) // instant
	mid := h.percentileValue(0.6)  // fast
	low := h.percentileValue(0.5)  // standard

	return high, mid, low
}

func max(a *big.Int, b *big.Int) *big.Int {
	if a.Cmp(b) >= 0 {
		return new(big.Int).Set(a)
	} else {
		return new(big.Int).Set(b)
	}
}
