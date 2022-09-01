package ethgas

import (
	"context"
	"math/big"
	"sort"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

var chainComputeStrategy = map[uint64]GasGaugeCompute{}

func init() {
	computeStandard := newGasGaugeStandard()
	computeArbitrum := newGasGaugeArbitrum()

	chainComputeStrategy[1] = computeStandard
	chainComputeStrategy[42161] = computeArbitrum // Arbitrum One
	chainComputeStrategy[42170] = computeArbitrum // Arbitrum Nova

	// NOTE: default strategy is for chain 1, so we only need to specify chain 1
	// and all the outliers
}

type GasGaugeCompute interface {
	ComputeSuggestedGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error)
}

type gasGaugeStandard struct {
	emaSlow, emaStandard, emaFast, emaInstant *EMA
}

func newGasGaugeStandard() *gasGaugeStandard {
	return &gasGaugeStandard{
		emaSlow:     NewEMA(0.5),
		emaStandard: NewEMA(0.5),
		emaFast:     NewEMA(0.5),
		emaInstant:  NewEMA(0.5),
	}
}

func (c *gasGaugeStandard) ComputeSuggestedGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error) {
	g := gasGauge

	txns := block.Transactions()
	if len(txns) == 0 {
		return nil, nil
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
			gasPrice = new(big.Int).Add(txn.GasTipCap(), block.BaseFee())
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
		return nil, nil
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
	c.emaSlow.Tick(slow)
	c.emaStandard.Tick(standard)
	c.emaFast.Tick(fast)
	c.emaInstant.Tick(instant)

	// compute final suggested gas price by averaging the samples
	// over a period of time
	sgp := SuggestedGasPrice{
		BlockNum:    block.Number(),
		BlockTime:   block.Time(),
		InstantWei:  new(big.Int).Set(c.emaInstant.Value()),
		FastWei:     new(big.Int).Set(c.emaFast.Value()),
		StandardWei: new(big.Int).Set(c.emaStandard.Value()),
		SlowWei:     new(big.Int).Set(c.emaSlow.Value()),
		Instant:     new(big.Int).Div(c.emaInstant.Value(), ONE_GWEI_BIG).Uint64(),
		Fast:        new(big.Int).Div(c.emaFast.Value(), ONE_GWEI_BIG).Uint64(),
		Standard:    new(big.Int).Div(c.emaStandard.Value(), ONE_GWEI_BIG).Uint64(),
		Slow:        new(big.Int).Div(c.emaSlow.Value(), ONE_GWEI_BIG).Uint64(),
	}

	return &sgp, nil
}

type gasGaugeArbitrum struct {
	emaSlow, emaStandard, emaFast, emaInstant *EMA
}

func newGasGaugeArbitrum() *gasGaugeArbitrum {
	return &gasGaugeArbitrum{
		emaSlow:     NewEMA(0.5),
		emaStandard: NewEMA(0.5),
		emaFast:     NewEMA(0.5),
		emaInstant:  NewEMA(0.5),
	}
}

func (c *gasGaugeArbitrum) ComputeSuggestedGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error) {
	panic("TODO")
	return nil, nil
}
