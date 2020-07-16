package ethcoder

import (
	"math/big"
	"testing"

	"github.com/arcadeum/ethkit/ethwallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

func TestTypedDataTypes(t *testing.T) {
	types := TypedDataTypes{
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
	typeHashHex := HexEncode(typeHash)
	assert.Equal(t, "0xb9d8c78acf9b987311de6c7b45bb6a9c8e1bf361fa7fd3467a2163f994c79500", typeHashHex)

	encodeType, err = types.EncodeType("Mail")
	assert.NoError(t, err)
	assert.Equal(t, "Mail(Person from,Person to,string contents,Asset asset)Asset(string name)Person(string name,address wallet)", encodeType)
}

func TestTypedDataCase1(t *testing.T) {
	verifyingContract := common.HexToAddress("0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC")

	typedData := &TypedData{
		Types: TypedDataTypes{
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
		Domain: TypedDataDomain{
			Name:              "Ether Mail",
			Version:           "1",
			ChainID:           big.NewInt(1),
			VerifyingContract: &verifyingContract,
		},
		Message: map[string]interface{}{
			"name":   "Bob",
			"wallet": common.HexToAddress("0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB"), // TODO: hmmmmmmpf.......... can we make this a string?

			// TODO: when unmarshalling a json string to a TypedData object, the "Message" format
			// will be of type string, so we'll need to parse string values, can try to use AbiUnmarshalStringValues instead
			// "wallet": "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
		},
	}

	domainHash, err := typedData.hashStruct("EIP712Domain", typedData.Domain.Map())
	assert.NoError(t, err)
	assert.Equal(t, "0xf2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090f", HexEncode(domainHash))

	digest, err := typedData.EncodeDigest()
	assert.NoError(t, err)
	assert.Equal(t, "0x0a94cf6625e5860fc4f330d75bcd0c3a4737957d2321d1a024540ab5320fe903", HexEncode(digest))

	// fmt.Println("===> digest", HexEncode(digest))

	// lets sign it..
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	assert.NoError(t, err)

	ethSigedTypedData, err := wallet.SignMessage(digest)
	ethSigedTypedDataHex := HexEncode(ethSigedTypedData)
	assert.NoError(t, err)

	assert.Equal(t,
		"0x842ed2d5c3bf97c4977ee84e600fec7d0f9c5e21d4090b5035a3ea650ec6127d18053e4aafb631de26eb3fd5d61e4a6f2d6a106ee8e3d8d5cb0c4571d06798741b",
		ethSigedTypedDataHex,
	)

	// validate signature
	valid, err := ethwallet.ValidateEthereumSignature(wallet.Address().Hex(), digest, ethSigedTypedDataHex)
	assert.NoError(t, err)
	assert.True(t, valid)
}
