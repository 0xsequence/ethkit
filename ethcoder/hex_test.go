package ethcoder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHexTrimLeadingZeros(t *testing.T) {
	v, err := HexTrimLeadingZeros("0x00000000001")
	assert.NoError(t, err)
	assert.Equal(t, "0x1", v)

	v, err = HexTrimLeadingZeros("0x000000000000")
	assert.NoError(t, err)
	assert.Equal(t, "0x0", v)
}

func TestHexTrimLeadingZerosMissingPrefix(t *testing.T) {
	for _, input := range []string{"", "0", "x", "00", "1234"} {
		t.Run(input, func(t *testing.T) {
			got, err := HexTrimLeadingZeros(input)
			assert.Error(t, err)
			assert.Empty(t, got)
		})
	}
}
