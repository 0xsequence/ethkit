package ethcoder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFunctionSignature(t *testing.T) {
	fnsig := FunctionSignature("balanceOf(address,uint256)")
	assert.Equal(t, "0x00fdd58e", fnsig)
}
