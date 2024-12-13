package ethcoder_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypedDataTypes(t *testing.T) {
	types := ethcoder.TypedDataTypes{
		"Person": {
			{Name: "name", Type: "string"},
			{Name: "wallet", Type: "address"},
		},
		"Mail": {
			{Name: "from", Type: "Person"},
			{Name: "to", Type: "Person"},
			{Name: "contents", Type: "string"},
			{Name: "asset", Type: "Asset"},
		},
		"Asset": {
			{Name: "name", Type: "string"},
		},
	}

	encodeType, err := types.EncodeType("Person")
	assert.NoError(t, err)
	assert.Equal(t, "Person(string name,address wallet)", encodeType)

	typeHash, _ := types.TypeHash("Person")
	typeHashHex := ethcoder.HexEncode(typeHash)
	assert.Equal(t, "0xb9d8c78acf9b987311de6c7b45bb6a9c8e1bf361fa7fd3467a2163f994c79500", typeHashHex)

	encodeType, err = types.EncodeType("Mail")
	assert.NoError(t, err)
	assert.Equal(t, "Mail(Person from,Person to,string contents,Asset asset)Asset(string name)Person(string name,address wallet)", encodeType)
}

func TestTypedDataCase1(t *testing.T) {
	verifyingContract := common.HexToAddress("0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC")

	typedData := &ethcoder.TypedData{
		Types: ethcoder.TypedDataTypes{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Person": {
				{Name: "name", Type: "string"},
				{Name: "wallet", Type: "address"},
			},
		},
		PrimaryType: "Person",
		Domain: ethcoder.TypedDataDomain{
			Name:              "Ether Mail",
			Version:           "1",
			ChainID:           big.NewInt(1),
			VerifyingContract: &verifyingContract,
		},
		Message: map[string]interface{}{
			"name": "Bob",
			// "wallet": common.HexToAddress("0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB"), // NOTE: passing common.Address object works too
			"wallet": "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
		},
	}

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	assert.NoError(t, err)
	assert.Equal(t, "0xf2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090f", ethcoder.HexEncode(domainHash))

	digest, _, err := typedData.Encode()
	assert.NoError(t, err)
	assert.Equal(t, "0x0a94cf6625e5860fc4f330d75bcd0c3a4737957d2321d1a024540ab5320fe903", ethcoder.HexEncode(digest))

	fmt.Println("===> digest", ethcoder.HexEncode(digest))

	// lets sign it..
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	assert.NoError(t, err)

	// TODO: this is wrong.. we need wallet.SignTypedData(digest).. or wallet.SignData(digest) wherre digest is fully encoded with prefix, etc.

	ethSigedTypedData, err := wallet.SignMessage([]byte(digest))
	ethSigedTypedDataHex := ethcoder.HexEncode(ethSigedTypedData)
	assert.NoError(t, err)

	assert.Equal(t,
		"0x842ed2d5c3bf97c4977ee84e600fec7d0f9c5e21d4090b5035a3ea650ec6127d18053e4aafb631de26eb3fd5d61e4a6f2d6a106ee8e3d8d5cb0c4571d06798741b",
		ethSigedTypedDataHex,
	)

	// recover / validate signature
	valid, err := ethwallet.ValidateEthereumSignature(wallet.Address().Hex(), digest, ethSigedTypedDataHex)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestTypedDataCase2(t *testing.T) {
	verifyingContract := common.HexToAddress("0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC")

	typedData := &ethcoder.TypedData{
		Types: ethcoder.TypedDataTypes{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Person": {
				{Name: "name", Type: "string"},
				{Name: "wallet", Type: "address"},
				{Name: "count", Type: "uint8"},
			},
		},
		PrimaryType: "Person",
		Domain: ethcoder.TypedDataDomain{
			Name:              "Ether Mail",
			Version:           "1",
			ChainID:           big.NewInt(1),
			VerifyingContract: &verifyingContract,
		},
		Message: map[string]interface{}{
			"name": "Bob",
			// "wallet": common.HexToAddress("0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB"), // NOTE: passing common.Address object works too
			"wallet": "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
			"count":  uint8(4),
		},
	}

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	assert.NoError(t, err)
	assert.Equal(t, "0xf2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090f", ethcoder.HexEncode(domainHash))

	digest, _, err := typedData.Encode()
	assert.NoError(t, err)
	assert.Equal(t, "0x2218fda59750be7bb9e5dfb2b49e4ec000dc2542862c5826f1fe980d6d727e95", ethcoder.HexEncode(digest))

	// fmt.Println("===> digest", HexEncode(digest))

}

func TestTypedDataFromJSON(t *testing.T) {
	typedDataJson := `{
		"types": {
			"EIP712Domain": [
				{"name": "name", "type": "string"},
				{"name": "version", "type": "string"},
				{"name": "chainId", "type": "uint256"},
				{"name": "verifyingContract", "type": "address"}
			],
			"Person": [
				{"name": "name", "type": "string"},
				{"name": "wallet", "type": "address"},
				{"name": "count", "type": "uint8"}
			]
		},
		"primaryType": "Person",
		"domain": {
			"name": "Ether Mail",
			"version": "1",
			"chainId": 1,
			"verifyingContract": "0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC"
		},
		"message": {
			"name": "Bob",
			"wallet": "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
			"count": 4
		}
	}`

	typedData, err := ethcoder.TypedDataFromJSON(typedDataJson)
	require.NoError(t, err)

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	require.NoError(t, err)
	require.Equal(t, "0xf2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090f", ethcoder.HexEncode(domainHash))

	digest, typedDataEncoded, err := typedData.Encode()
	require.NoError(t, err)
	require.Equal(t, "0x2218fda59750be7bb9e5dfb2b49e4ec000dc2542862c5826f1fe980d6d727e95", ethcoder.HexEncode(digest))
	require.Equal(t, "0x1901f2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090ff5117e79519388f3d62844df1325ebe783523d9db9762c50fa78a60400a20b5b", ethcoder.HexEncode(typedDataEncoded))

	// Sign and validate
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	require.NoError(t, err)

	ethSigedTypedData, typedDataEncodedOut, err := wallet.SignTypedData(typedData)
	ethSigedTypedDataHex := ethcoder.HexEncode(ethSigedTypedData)
	require.NoError(t, err)
	require.Equal(t, typedDataEncoded, typedDataEncodedOut)

	// NOTE: this signature and above method has been compared against ethers v6 test
	require.Equal(t,
		"0x296c98bed8f3fd7ea96f55ca8148b4d092cbada953c8d9205b2fff759461ab4e6d6db0b78833b954684900530caeee9aaef8e42dfd8439a3fa107e910b57e2cc1b",
		ethSigedTypedDataHex,
	)

	// recover / validate signature
	valid, err := ethwallet.ValidateEthereumSignature(wallet.Address().Hex(), typedDataEncodedOut, ethSigedTypedDataHex)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestTypedDataFromJSONPart2(t *testing.T) {
	// NOTE: we omit the EIP712Domain type definition because it will
	// automatically be added by the library if its not specified
	typedDataJson := `{
		"types": {
			"Person": [
				{ "name": "name", "type": "string" },
				{ "name": "wallets", "type": "address[]" }
			],
			"Mail": [
				{ "name": "from", "type": "Person" },
				{ "name": "to", "type": "Person[]" },
				{ "name": "contents", "type": "string" }
			],
			"Group": [
				{ "name": "name", "type": "string" },
				{ "name": "members", "type": "Person[]" }
			]
		},
		"domain": {
			"name": "Ether Mail",
			"version": "1",
			"chainId": 1,
			"verifyingContract": "0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC"
		},
		"primaryType": "Mail",
		"message": {
			"from": {
				"name": "Cow",
				"wallets": [
					"0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826",
					"0xDeaDbeefdEAdbeefdEadbEEFdeadbeEFdEaDbeeF"
				]
			},
			"to": [{
				"name": "Bob",
				"wallets": [
					"0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
					"0xB0BdaBea57B0BDABeA57b0bdABEA57b0BDabEa57",
					"0xB0B0b0b0b0b0B000000000000000000000000000"
				]
			}],
			"contents": "Hello, Bob!"
		}
	}`

	typedData, err := ethcoder.TypedDataFromJSON(typedDataJson)
	require.NoError(t, err)

	spew.Dump(typedData)

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	require.NoError(t, err)
	require.Equal(t, "0xf2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090f", ethcoder.HexEncode(domainHash))

	fromArg, ok := typedData.Message["from"].(map[string]interface{})
	require.True(t, ok)
	personHash, err := typedData.HashStruct("Person", fromArg)
	require.NoError(t, err)
	require.Equal(t, "0x12345", ethcoder.HexEncode(personHash))

	digest, typedDataEncoded, err := typedData.Encode()
	require.NoError(t, err)
	require.Equal(t, "0x2218fda59750be7bb9e5dfb2b49e4ec000dc2542862c5826f1fe980d6d727e95", ethcoder.HexEncode(digest))
	require.Equal(t, "0x1901f2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090ff5117e79519388f3d62844df1325ebe783523d9db9762c50fa78a60400a20b5b", ethcoder.HexEncode(typedDataEncoded))

	return

	// Sign and validate
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	require.NoError(t, err)

	ethSigedTypedData, typedDataEncodedOut, err := wallet.SignTypedData(typedData)
	ethSigedTypedDataHex := ethcoder.HexEncode(ethSigedTypedData)
	require.NoError(t, err)
	require.Equal(t, typedDataEncoded, typedDataEncodedOut)

	// NOTE: this signature and above method has been compared against ethers v6 test
	require.Equal(t,
		"0x296c98bed8f3fd7ea96f55ca8148b4d092cbada953c8d9205b2fff759461ab4e6d6db0b78833b954684900530caeee9aaef8e42dfd8439a3fa107e910b57e2cc1b",
		ethSigedTypedDataHex,
	)

	// recover / validate signature
	valid, err := ethwallet.ValidateEthereumSignature(wallet.Address().Hex(), typedDataEncodedOut, ethSigedTypedDataHex)
	require.NoError(t, err)
	require.True(t, valid)
}
