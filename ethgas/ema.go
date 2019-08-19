package ethgas

import (
	"math/big"
)

// NewEMA(decay) returns a new exponential moving average. It weighs new values more than
// existing values according to the decay. For example: NewEMA(0.05) would give 5% weight
// to new values and 95% weight to past values. Common to use 2/(selected time period+1).
func NewEMA(decay float64) *EMA {
	return &EMA{decay: big.NewFloat(decay)}
}

type EMA struct {
	decay *big.Float
	value *big.Int
}

func (ema *EMA) Tick(price *big.Int) *big.Int {
	if ema.value == nil {
		ema.value = price
	}
	current := new(big.Float).Mul(new(big.Float).SetInt(price), ema.decay)
	past := new(big.Float).Mul(
		new(big.Float).Sub(big.NewFloat(1), ema.decay),
		new(big.Float).SetInt(ema.value),
	)
	next := new(big.Float).Add(current, past)
	ema.value, _ = next.Int(nil)
	return ema.value
}

func (ema *EMA) Value() *big.Int {
	return ema.value
}
