package ethwallet_test

import (
	"testing"

	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/stretchr/testify/assert"
)

func TestHDNodeMnemonicAndEntropy(t *testing.T) {
	testMnemonic := "outdoor sentence roast truly flower surface power begin ocean silent debate funny"

	entropy, err := ethwallet.MnemonicToEntropy(testMnemonic)
	assert.NoError(t, err)
	assert.NotEmpty(t, entropy)

	mnemonic, err := ethwallet.EntropyToMnemonic(entropy)
	assert.NoError(t, err)
	assert.Equal(t, testMnemonic, mnemonic)
}

func TestHDNode(t *testing.T) {
	hdnode, err := ethwallet.NewHDNodeFromRandomEntropy(ethwallet.EntropyBitSize12WordMnemonic, nil)
	assert.NoError(t, err)
	assert.NotNil(t, hdnode)
	assert.NotEmpty(t, hdnode.Mnemonic())
	assert.NotEqual(t, hdnode.Address().Hex(), "0x0000000000000000000000000000000000000000")

	hdnode2, err := ethwallet.NewHDNodeFromMnemonic(hdnode.Mnemonic(), nil)
	assert.NoError(t, err)
	assert.NotNil(t, hdnode2)
	assert.Equal(t, hdnode.Address().Hex(), hdnode2.Address().Hex())
}

func TestHDNodeDerivationPath(t *testing.T) {
	testMnemonic := "outdoor sentence roast truly flower surface power begin ocean silent debate funny"

	hdnode, err := ethwallet.NewHDNodeFromMnemonic(testMnemonic, nil)
	assert.NoError(t, err)
	assert.NotNil(t, hdnode)
	assert.Equal(t, "0xe0C9828dee3411A28CcB4bb82a18d0aAd24489E0", hdnode.Address().Hex())
	assert.Equal(t, testMnemonic, hdnode.Mnemonic())

	err = hdnode.DeriveAccountIndex(1)
	assert.NoError(t, err)
	assert.Equal(t, "0x9e02d584c27Ec74f832154985046C0f3c5E0f724", hdnode.Address().Hex())

	err = hdnode.DeriveAccountIndex(0)
	assert.NoError(t, err)
	assert.Equal(t, "0xe0C9828dee3411A28CcB4bb82a18d0aAd24489E0", hdnode.Address().Hex())
}

func TestHDNodeDerivationFailsWithInvalidMnemonic(t *testing.T) {
	testInvalidMnemonics := []string{
		// one word too short
		"sentence roast truly flower surface power begin ocean silent debate funny",

		// first word is jamaican patois, not valid bip39 word
		"wahgwaan sentence roast truly flower surface power begin ocean silent debate funny",

		// one word too long
		"outdoor sentence roast truly flower surface power begin ocean silent debate funny funny",
	}

	for _, mnemonic := range testInvalidMnemonics {
		hdnode, err := ethwallet.NewHDNodeFromMnemonic(mnemonic, nil)
		assert.Errorf(t, err, "Expected error for invalid mnemonic %v", mnemonic)
		assert.Nilf(t, hdnode, "Expected nil hdnode for invalid mnemonic %v", mnemonic)
	}
}
