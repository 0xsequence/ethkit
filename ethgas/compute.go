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
	ComputeSuggestedActualGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error)
	ComputeSuggestedProposedGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error)
}

type gasGaugeStandard struct {
	emaActualSlow, emaActualStandard, emaActualFast, emaActualInstant         *EMA
	emaProposedSlow, emaProposedStandard, emaProposedFast, emaProposedInstant *EMA
}

func newGasGaugeStandard() *gasGaugeStandard {
	return &gasGaugeStandard{
		emaActualSlow:       NewEMA(0.5),
		emaActualStandard:   NewEMA(0.5),
		emaActualFast:       NewEMA(0.5),
		emaActualInstant:    NewEMA(0.5),
		emaProposedSlow:     NewEMA(0.5),
		emaProposedStandard: NewEMA(0.5),
		emaProposedFast:     NewEMA(0.5),
		emaProposedInstant:  NewEMA(0.5),
	}
}

func (c *gasGaugeStandard) ComputeSuggestedActualGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error) {
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
	c.emaActualSlow.Tick(slow)
	c.emaActualStandard.Tick(standard)
	c.emaActualFast.Tick(fast)
	c.emaActualInstant.Tick(instant)

	// compute final suggested gas price by averaging the samples
	// over a period of time
	sgp := SuggestedGasPrice{
		BlockNum:    block.Number(),
		BlockTime:   block.Time(),
		InstantWei:  new(big.Int).Set(c.emaActualInstant.Value()),
		FastWei:     new(big.Int).Set(c.emaActualFast.Value()),
		StandardWei: new(big.Int).Set(c.emaActualStandard.Value()),
		SlowWei:     new(big.Int).Set(c.emaActualSlow.Value()),
		Instant:     new(big.Int).Div(c.emaActualInstant.Value(), ONE_GWEI_BIG).Uint64(),
		Fast:        new(big.Int).Div(c.emaActualFast.Value(), ONE_GWEI_BIG).Uint64(),
		Standard:    new(big.Int).Div(c.emaActualStandard.Value(), ONE_GWEI_BIG).Uint64(),
		Slow:        new(big.Int).Div(c.emaActualSlow.Value(), ONE_GWEI_BIG).Uint64(),
	}

	return &sgp, nil
}

func (c *gasGaugeStandard) ComputeSuggestedProposedGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error) {
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
			gasPrice = txn.GasFeeCap()
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
	c.emaProposedSlow.Tick(slow)
	c.emaProposedStandard.Tick(standard)
	c.emaProposedFast.Tick(fast)
	c.emaProposedInstant.Tick(instant)

	// compute final suggested gas price by averaging the samples
	// over a period of time
	sgp := SuggestedGasPrice{
		BlockNum:    block.Number(),
		BlockTime:   block.Time(),
		InstantWei:  new(big.Int).Set(c.emaProposedInstant.Value()),
		FastWei:     new(big.Int).Set(c.emaProposedFast.Value()),
		StandardWei: new(big.Int).Set(c.emaProposedStandard.Value()),
		SlowWei:     new(big.Int).Set(c.emaProposedSlow.Value()),
		Instant:     new(big.Int).Div(c.emaProposedInstant.Value(), ONE_GWEI_BIG).Uint64(),
		Fast:        new(big.Int).Div(c.emaProposedFast.Value(), ONE_GWEI_BIG).Uint64(),
		Standard:    new(big.Int).Div(c.emaProposedStandard.Value(), ONE_GWEI_BIG).Uint64(),
		Slow:        new(big.Int).Div(c.emaProposedSlow.Value(), ONE_GWEI_BIG).Uint64(),
	}

	return &sgp, nil
}

type gasGaugeArbitrum struct {
	emaActualSlow, emaActualStandard, emaActualFast, emaActualInstant         *EMA
	emaProposedSlow, emaProposedStandard, emaProposedFast, emaProposedInstant *EMA
}

func newGasGaugeArbitrum() *gasGaugeArbitrum {
	return &gasGaugeArbitrum{
		emaActualSlow:       NewEMA(0.5),
		emaActualStandard:   NewEMA(0.5),
		emaActualFast:       NewEMA(0.5),
		emaActualInstant:    NewEMA(0.5),
		emaProposedSlow:     NewEMA(0.5),
		emaProposedStandard: NewEMA(0.5),
		emaProposedFast:     NewEMA(0.5),
		emaProposedInstant:  NewEMA(0.5),
	}
}

func (c *gasGaugeArbitrum) ComputeSuggestedActualGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error) {
	g := gasGauge

	txns := block.Transactions()
	if len(txns) == 0 {
		return nil, nil
	}

	var gasPrices []*big.Int
	for range txns {
		gasPrices = append(gasPrices, block.BaseFee())
	}

	// Case if there are no gas prices sampled from any of the transactions, lets
	// use the block's base fee once
	if len(gasPrices) == 0 {
		gasPrices = append(gasPrices, block.BaseFee())
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
	c.emaActualSlow.Tick(slow)
	c.emaActualStandard.Tick(standard)
	c.emaActualFast.Tick(fast)
	c.emaActualInstant.Tick(instant)

	// compute final suggested gas price by averaging the samples
	// over a period of time
	sgp := SuggestedGasPrice{
		BlockNum:    block.Number(),
		BlockTime:   block.Time(),
		InstantWei:  new(big.Int).Set(c.emaActualInstant.Value()),
		FastWei:     new(big.Int).Set(c.emaActualFast.Value()),
		StandardWei: new(big.Int).Set(c.emaActualStandard.Value()),
		SlowWei:     new(big.Int).Set(c.emaActualSlow.Value()),
		Instant:     new(big.Int).Div(c.emaActualInstant.Value(), ONE_GWEI_BIG).Uint64(),
		Fast:        new(big.Int).Div(c.emaActualFast.Value(), ONE_GWEI_BIG).Uint64(),
		Standard:    new(big.Int).Div(c.emaActualStandard.Value(), ONE_GWEI_BIG).Uint64(),
		Slow:        new(big.Int).Div(c.emaActualSlow.Value(), ONE_GWEI_BIG).Uint64(),
	}

	return &sgp, nil
}

func (c *gasGaugeArbitrum) ComputeSuggestedProposedGasPrice(ctx context.Context, gasGauge *GasGauge, block *ethmonitor.Block) (*SuggestedGasPrice, error) {
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
			gasPrice = txn.GasFeeCap()
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
	c.emaProposedSlow.Tick(slow)
	c.emaProposedStandard.Tick(standard)
	c.emaProposedFast.Tick(fast)
	c.emaProposedInstant.Tick(instant)

	// compute final suggested gas price by averaging the samples
	// over a period of time
	sgp := SuggestedGasPrice{
		BlockNum:    block.Number(),
		BlockTime:   block.Time(),
		InstantWei:  new(big.Int).Set(c.emaProposedInstant.Value()),
		FastWei:     new(big.Int).Set(c.emaProposedFast.Value()),
		StandardWei: new(big.Int).Set(c.emaProposedStandard.Value()),
		SlowWei:     new(big.Int).Set(c.emaProposedSlow.Value()),
		Instant:     new(big.Int).Div(c.emaProposedInstant.Value(), ONE_GWEI_BIG).Uint64(),
		Fast:        new(big.Int).Div(c.emaProposedFast.Value(), ONE_GWEI_BIG).Uint64(),
		Standard:    new(big.Int).Div(c.emaProposedStandard.Value(), ONE_GWEI_BIG).Uint64(),
		Slow:        new(big.Int).Div(c.emaProposedSlow.Value(), ONE_GWEI_BIG).Uint64(),
	}

	return &sgp, nil
}
