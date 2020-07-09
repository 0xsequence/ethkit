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
