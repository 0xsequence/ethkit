package ethgas

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/0xsequence/ethkit/ethmonitor"
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
		return nil, fmt.Errorf("minGasPriceInWei cannot be 0, pass at least 1")
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

	chainID, err := g.ethMonitor.Provider().ChainID(context.Background())
	if err != nil {
		return err
	}

	computeStrategy, ok := chainComputeStrategy[chainID.Uint64()]
	if !ok {
		computeStrategy = chainComputeStrategy[1]
	}

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

			sgp, err := computeStrategy.ComputeSuggestedGasPrice(g.ctx, g, latestBlock)
			if err != nil {
				g.log.Errorf("gas gauge compute error: %s", err.Error())
				continue
			}
			if sgp == nil {
				continue // skip and do nothing
			}

			g.suggestedGasPriceUpdated.L.Lock()
			g.suggestedGasPrice = *sgp
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
