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

func TestWalletSignMessageFromPrivateKey(t *testing.T) {
	wallet, err := ethwallet.NewWalletFromPrivateKey("3c121e5b2c2b2426f386bfc0257820846d77610c20e0fd4144417fb8fd79bfb8")
	assert.NoError(t, err)

	address := wallet.Address()
	assert.NoError(t, err)
	assert.Equal(t, "0x95a7D93FEf729ed829C761FF0e035BB6Dd2c7052", address.String())

	sig, err := wallet.SignMessage([]byte("hi"))
	assert.NoError(t, err)

	sigHash := hexutil.Encode(sig)
	expected := "0x14c0b4cbb654b3da1140cdf5c000bfbf5db810f5a7fb339dd4514230d20e1bae4bf9ab78b6431b975260676a020cb4f7c164161776ee6fedbce39eb4103b257f1c"
	assert.Equal(t, expected, sigHash)

	// Lets validate the signature for good measure
	valid, err := ethwallet.ValidateEthereumSignature(address.String(), []byte("hi"), sigHash)
	assert.NoError(t, err)
	assert.True(t, valid)
}
