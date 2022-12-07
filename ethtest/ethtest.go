package ethtest

import (
	"context"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/stretchr/testify/assert"
)

// RandomSeed will generate a random seed
func RandomSeed() uint64 {
	rand.Seed(time.Now().UnixNano())
	return rand.Uint64()
}

func ETHValue(ether float64) *big.Int {
	x := big.NewInt(10)
	x.Exp(x, big.NewInt(15), nil)
	n := big.NewInt(int64(ether * 1000))
	return n.Mul(n, x)
}

func ETHValueBigInt(ether *big.Int) *big.Int {
	oneEth := big.NewInt(10)
	oneEth.Exp(oneEth, big.NewInt(18), nil)
	return ether.Mul(ether, oneEth)
}

func GetBalance(t *testing.T, wallet *ethwallet.Wallet) *big.Int {
	balance, err := wallet.GetBalance(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, balance)
	return balance
}
