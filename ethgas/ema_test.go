package ethgas

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEMA(t *testing.T) {
	tt := []struct {
		decay    float64
		values   []int64
		expected int64
	}{
		{
			decay:    0.1,
			values:   []int64{1, 1, 1},
			expected: 1,
		},
		{
			decay:    0.5,
			values:   []int64{300, 200, 100},
			expected: 175,
		},
		{
			decay:    0.1818,
			values:   []int64{2227, 2219, 2208, 2217, 2218, 2213, 2223, 2243, 2224},
			expected: 2222,
		},
	}
	for _, tc := range tt {
		ema := NewEMA(tc.decay)
		for _, v := range tc.values {
			ema.Tick(big.NewInt(v))
		}
		assert.Equal(t, tc.expected, ema.Value().Int64())
	}
}
