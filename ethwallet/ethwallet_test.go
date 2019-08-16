package ethwallet_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/horizon-games/ethkit/ethcoder"
	"github.com/horizon-games/ethkit/ethwallet"
	"github.com/stretchr/testify/assert"
)

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
	valid, err := ethwallet.ValidateEthereumSignature(address.String(), "hi", sigHash)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestWalletSignedTypedData(t *testing.T) {
	// EIP-712 Domain Hash
	DOMAIN_SEPARATOR_TYPEHASH := "0x035aff83d86937d35b32e04f0ddc6ff469290eef2f1b692d8a815c89404d4749"
	DOMAIN_SEPARATOR_TYPEHASH_BYTES32, err := ethcoder.HexDecodeBytes32(DOMAIN_SEPARATOR_TYPEHASH)
	assert.NoError(t, err)

	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	assert.NoError(t, err)

	address := wallet.Address()
	assert.NoError(t, err)
	assert.Equal(t, "0xb59ba5A13f0fb106EA6094a1F69786AA69be1424", address.String())

	{
		sig, err := wallet.SignMessage([]byte("hi"))
		assert.NoError(t, err)
		ethersExpected := "0xebe541eda2d15b7abc8ff48a052be03b6ae8f05c1a88ac0483af741a896ab75945ed5dddc8a839ed1e78f0591d8878181c5d00a79d7a4f0778b19c34dee6e8a41c"
		assert.Equal(t, ethersExpected, ethcoder.HexEncode(sig))
	}

	{
		mockContractAddressString := "0x61b2eFA7631511c2c12c036e905fEA10dc4fA6A7"
		mockContractAddress := common.HexToAddress(mockContractAddressString)

		// First lets verify domainHash arg
		domainHashPack, err := ethcoder.SolidityPack([]string{"bytes32", "address"}, []interface{}{DOMAIN_SEPARATOR_TYPEHASH_BYTES32, mockContractAddress})
		assert.NoError(t, err)
		domainHash := ethcoder.Keccak256(domainHashPack)
		domainHashHex := ethcoder.HexEncode(domainHash)
		ethersExpected := "0x8256e199895a50dde8d404fce0f88f8ec9483c4c346b20e831a0593bfc490f01"
		assert.Equal(t, ethersExpected, domainHashHex)

		// Now, ethSignedTypeData
		hashStruct := ethcoder.Keccak256(mockContractAddress.Bytes()) // random value for now..

		sig, err := wallet.SignTypedData(ethcoder.BytesToBytes32(domainHash), ethcoder.BytesToBytes32(hashStruct))
		assert.NoError(t, err)
		ethersExpected = "0xe263b4e32306e85b2f4f177140bd34fbbad2a06f9f54e3f85b836dd08618e9a3052843d10a7f749007c710cc7acaea8663886fb72afe708157c9019eddb271011c02"
		assert.Equal(t, ethersExpected, ethcoder.HexEncode(sig))
	}
}
