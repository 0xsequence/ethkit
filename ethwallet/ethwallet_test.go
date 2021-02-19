package ethwallet_test

import (
	"testing"

	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
)

func TestWalletRandom(t *testing.T) {
	wallet, err := ethwallet.NewWalletFromRandomEntropy()
	assert.NoError(t, err)
	assert.NotNil(t, wallet)
}

func TestWalletSignMessage(t *testing.T) {
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	assert.NoError(t, err)

	address := wallet.Address()
	assert.NoError(t, err)
	assert.Equal(t, "0xb59ba5A13f0fb106EA6094a1F69786AA69be1424", address.String())

	sig, err := wallet.SignMessage([]byte("hi"))
	assert.NoError(t, err)

	sigHash := hexutil.Encode(sig)
	expected := "0xebe541eda2d15b7abc8ff48a052be03b6ae8f05c1a88ac0483af741a896ab75945ed5dddc8a839ed1e78f0591d8878181c5d00a79d7a4f0778b19c34dee6e8a41c"
	assert.Equal(t, expected, sigHash)

	// Lets validate the signature for good measure
	valid, err := ethwallet.ValidateEthereumSignature(address.String(), []byte("hi"), sigHash)
	assert.NoError(t, err)
	assert.True(t, valid)
}
