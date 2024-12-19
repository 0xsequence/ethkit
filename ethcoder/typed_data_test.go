package ethcoder_test

import (
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
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
			"name":   "Bob",
			"wallet": common.HexToAddress("0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB"),
		},
	}

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	assert.NoError(t, err)
	assert.Equal(t, "0xf2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090f", ethcoder.HexEncode(domainHash))

	digest, _, err := typedData.Encode()
	assert.NoError(t, err)
	assert.Equal(t, "0x0a94cf6625e5860fc4f330d75bcd0c3a4737957d2321d1a024540ab5320fe903", ethcoder.HexEncode(digest))

	// fmt.Println("===> digest", ethcoder.HexEncode(digest))

	// lets sign it..
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	assert.NoError(t, err)

	ethSigedTypedData, encodedTypeData, err := wallet.SignTypedData(typedData)
	ethSigedTypedDataHex := ethcoder.HexEncode(ethSigedTypedData)
	assert.NoError(t, err)

	assert.Equal(t,
		"0x07cc7c723b24733e11494438927012ec9b086e8edcb06022231710988ff7e54c45b0bb8911b1e06d322eb24b919f2a479e3062fee75ce57c1f7d7fc16c371fa81b",
		ethSigedTypedDataHex,
	)

	// recover / validate signature
	valid, err := ethwallet.ValidateEthereumSignature(wallet.Address().Hex(), encodedTypeData, ethSigedTypedDataHex)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestTypedDataCase2(t *testing.T) {
	verifyingContract := common.HexToAddress("0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC")
	salt := common.HexToHash("0x1122112211221122112211221122112211221122112211221122112211221122")

	typedData := &ethcoder.TypedData{
		Types: ethcoder.TypedDataTypes{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
				{Name: "salt", Type: "bytes32"},
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
			Salt:              &salt,
		},
		Message: map[string]interface{}{
			"name":   "Bob",
			"wallet": common.HexToAddress("0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB"),
			"count":  uint8(4),
		},
	}

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	require.NoError(t, err)
	require.Equal(t, "0x7f94cc8efb643110a5cd5c94f08051bb7d6203f3f457a43d7f3138a219a1ea6f", ethcoder.HexEncode(domainHash))

	_, encodedTypeData, err := typedData.Encode()
	require.NoError(t, err)
	require.Equal(t, "0x19017f94cc8efb643110a5cd5c94f08051bb7d6203f3f457a43d7f3138a219a1ea6ff5117e79519388f3d62844df1325ebe783523d9db9762c50fa78a60400a20b5b", ethcoder.HexEncode(encodedTypeData))

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

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	require.NoError(t, err)
	require.Equal(t, "0xf2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090f", ethcoder.HexEncode(domainHash))

	personTypeHash, err := typedData.Types.TypeHash("Person")
	require.NoError(t, err)
	require.Equal(t, "0xfabfe1ed996349fc6027709802be19d047da1aa5d6894ff5f6486d92db2e6860", ethcoder.HexEncode(personTypeHash))

	fromArg, ok := typedData.Message["from"].(map[string]interface{})
	require.True(t, ok)
	personHashStruct, err := typedData.HashStruct("Person", fromArg)
	require.NoError(t, err)
	require.Equal(t, "0x9b4846dd48b866f0ac54d61b9b21a9e746f921cefa4ee94c4c0a1c49c774f67f", ethcoder.HexEncode(personHashStruct))

	mailHashStruct, err := typedData.HashStruct("Mail", typedData.Message)
	require.NoError(t, err)
	require.Equal(t, "0xeb4221181ff3f1a83ea7313993ca9218496e424604ba9492bb4052c03d5c3df8", ethcoder.HexEncode(mailHashStruct))

	digest, typedDataEncoded, err := typedData.Encode()
	require.NoError(t, err)
	require.Equal(t, "0xa85c2e2b118698e88db68a8105b794a8cc7cec074e89ef991cb4f5f533819cc2", ethcoder.HexEncode(digest))
	require.Equal(t, "0x1901f2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090feb4221181ff3f1a83ea7313993ca9218496e424604ba9492bb4052c03d5c3df8", ethcoder.HexEncode(typedDataEncoded))

	// Sign and validate
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	require.NoError(t, err)

	ethSigedTypedData, typedDataEncodedOut, err := wallet.SignTypedData(typedData)
	ethSigedTypedDataHex := ethcoder.HexEncode(ethSigedTypedData)
	require.NoError(t, err)
	require.Equal(t, typedDataEncoded, typedDataEncodedOut)

	// NOTE: this signature and above method has been compared against ethers v6 test
	require.Equal(t,
		"0xafd9e7d3b912a9ca989b622837ab92a8616446e6a517c486de5745dda166152f2d40f1d62593da438a65b58deacfdfbbeb7bbce2a12056815b19c678c563cc311c",
		ethSigedTypedDataHex,
	)

	// recover / validate signature
	valid, err := ethwallet.ValidateEthereumSignature(wallet.Address().Hex(), typedDataEncodedOut, ethSigedTypedDataHex)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestTypedDataFromJSONPart3(t *testing.T) {
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
				{"name": "count", "type": "uint8"},
				{"name": "data", "type": "bytes"},
				{"name": "hash", "type": "bytes32"}
			],
			"Blah": [
				{"name": "name", "type": "string"},
				{"name": "another", "type": "address"}
			]
		},
		"domain": {
			"name": "Ether Mail",
			"version": "1",
			"chainId": 1,
			"verifyingContract": "0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC"
		},
		"message": {
			"name": "Bob",
			"wallet": "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
			"count": 4,
			"data": "0x1234567890abcdef",
			"hash": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
		}
	}`

	typedData, err := ethcoder.TypedDataFromJSON(typedDataJson)
	require.NoError(t, err)

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	require.NoError(t, err)
	require.Equal(t, "0xf2cee375fa42b42143804025fc449deafd50cc031ca257e0b194a650a912090f", ethcoder.HexEncode(domainHash))

	// Sign and validate
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	require.NoError(t, err)

	ethSigedTypedData, typedDataEncodedOut, err := wallet.SignTypedData(typedData)
	ethSigedTypedDataHex := ethcoder.HexEncode(ethSigedTypedData)
	require.NoError(t, err)

	// NOTE: this signature and above method has been compared against ethers v6 test
	require.Equal(t,
		"0x5781ecfd09e949db2470bba3635a8040752429c43e605daa74c81e6c81d49953318d00ef44e055221cd87cc05907de1400b5f8f1a6f2e44869cddd3bc82c8ec51b",
		ethSigedTypedDataHex,
	)

	// recover / validate signature
	valid, err := ethwallet.ValidateEthereumSignature(wallet.Address().Hex(), typedDataEncodedOut, ethSigedTypedDataHex)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestTypedDataFromJSONPart4(t *testing.T) {
	typedDataJson := `{
		"types": {
			"EIP712Domain": [
				{ "name": "name", "type": "string" },
				{ "name": "version", "type": "string" },
				{ "name": "chainId", "type": "uint256" },
				{ "name": "verifyingContract", "type": "address" },
				{ "name": "salt", "type": "bytes32" }
			],
			"ExampleMessage": [
				{ "name": "message", "type": "string" },
				{ "name": "value", "type": "uint256" },
				{ "name": "from", "type": "address" },
				{ "name": "to", "type": "address" }
			]
		},
		"domain": {
			"name": "EIP712Example",
			"version": "1",
			"chainId": 5,
			"verifyingContract": "0xc0ffee254729296a45a3885639AC7E10F9d54979",
			"salt": "0x70736575646f2d72616e646f6d2076616c756500000000000000000000000000"
		},
		"message": {
			"message": "Test message",
			"value": 10000,
			"from": "0xc0ffee254729296a45a3885639AC7E10F9d54979",
			"to": "0xc0ffee254729296a45a3885639AC7E10F9d54979"
		}
	}`

	typedData, err := ethcoder.TypedDataFromJSON(typedDataJson)
	require.NoError(t, err)

	domainHash, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	require.NoError(t, err)
	require.Equal(t, "0xe073fe030277efdf89d39322ac9321f17774fce4f686e14ff161942bbae5fdcd", ethcoder.HexEncode(domainHash))

	// Sign and validate
	wallet, err := ethwallet.NewWalletFromMnemonic("dose weasel clever culture letter volume endorse used harvest ripple circle install")
	require.NoError(t, err)

	ethSigedTypedData, typedDataEncodedOut, err := wallet.SignTypedData(typedData)
	ethSigedTypedDataHex := ethcoder.HexEncode(ethSigedTypedData)
	require.NoError(t, err)

	// NOTE: this signature and above method has been compared against ethers v6 test
	require.Equal(t,
		"0xdf10e9cd68dc1464a8ddceeb2875be9e12c8706ce2467f5dec987f36731dac2a34130d46c5a96ca182ead8530bc0b16920487f5c2d378aaafd93c6e41617f3ea1b",
		ethSigedTypedDataHex,
	)

	// recover / validate signature
	valid, err := ethwallet.ValidateEthereumSignature(wallet.Address().Hex(), typedDataEncodedOut, ethSigedTypedDataHex)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestTypedDataFromJSONPart5(t *testing.T) {
	typedDataJson := `{
		"types": {
			"EIP712Domain": [
				{ "name": "name", "type": "string" },
				{ "name": "version", "type": "string" },
				{ "name": "chainId", "type": "uint256" },
				{ "name": "verifyingContract", "type": "address" },
				{ "name": "salt", "type": "bytes32" }
			],
			"ExampleMessage": [
				{ "name": "message", "type": "string" },
				{ "name": "value", "type": "uint256" },
				{ "name": "from", "type": "address" },
				{ "name": "to", "type": "address" }
			]
		},
		"domain": {
			"name": "EIP712Example",
			"version": "1",
			"chainId": "0x0f",
			"verifyingContract": "0xc0ffee254729296a45a3885639AC7E10F9d54979",
			"salt": "0x70736575646f2d72616e646f6d2076616c756500000000000000000000000000"
		},
		"message": {
			"message": "Test message",
			"value": "0x634abebe1d4da48b00000000000000000cde63753dad4f0f42f79ebef71ee924",
			"from": "0xc0ffee254729296a45a3885639AC7E10F9d54979",
			"to": "0xc0ffee254729296a45a3885639AC7E10F9d54979"
		}
	}`

	typedData, err := ethcoder.TypedDataFromJSON(typedDataJson)
	require.NoError(t, err)

	require.Equal(t, typedData.Domain.ChainID.Int64(), int64(15))
}

func TestTypedDataFromJSONPart6(t *testing.T) {
	typedDataJson := `{
		"domain": {
			"name": "Seaport",
			"version": "1.5",
			"chainId": 80002,
			"verifyingContract": "0x00000000000000adc04c56bf30ac9d3c0aaf14dc"
		},
		"message": {
			"conduitKey": "0xf3d63166f0ca56c3c1a3508fce03ff0cf3fb691e000000000000000000000000",
			"consideration": [
				{
					"endAmount": "1",
					"identifierOrCriteria": "1",
					"itemType": 3,
					"recipient": "0x033ccc543501e462a2d50b579845709ff21f2eb6",
					"startAmount": "1",
					"token": "0xb7d432df27ab7b2a1be636bd945e6cb63bc84feb"
				}
			],
			"counter": "0",
			"endTime": 1735219168,
			"offer": [
				{
					"endAmount": "1",
					"identifierOrCriteria": "0",
					"itemType": 1,
					"startAmount": "1",
					"token": "0x41e94eb019c0762f9bfcf9fb1e58725bfb0e7582"
				}
			],
			"offerer": "0x033ccc543501e462a2d50b579845709ff21f2eb6",
			"orderType": 1,
			"salt": "0x634abebe1d4da48b0000000000000000f6dad44ce6d8c81dcbf213906d353f0c",
			"startTime": 1734614365,
			"zone": "0x0000000000000000000000000000000000000000",
			"zoneHash": "0x0000000000000000000000000000000000000000000000000000000000000000"
		},
		"primaryType": "OrderComponents",
		"types": {
			"EIP712Domain": [
				{
					"name": "name",
					"type": "string"
				},
				{
					"name": "version",
					"type": "string"
				},
				{
					"name": "chainId",
					"type": "uint256"
				},
				{
					"name": "verifyingContract",
					"type": "address"
				}
			],
			"ConsiderationItem": [
				{
					"name": "itemType",
					"type": "uint8"
				},
				{
					"name": "token",
					"type": "address"
				},
				{
					"name": "identifierOrCriteria",
					"type": "uint256"
				},
				{
					"name": "startAmount",
					"type": "uint256"
				},
				{
					"name": "endAmount",
					"type": "uint256"
				},
				{
					"name": "recipient",
					"type": "address"
				}
			],
			"OfferItem": [
				{
					"name": "itemType",
					"type": "uint8"
				},
				{
					"name": "token",
					"type": "address"
				},
				{
					"name": "identifierOrCriteria",
					"type": "uint256"
				},
				{
					"name": "startAmount",
					"type": "uint256"
				},
				{
					"name": "endAmount",
					"type": "uint256"
				}
			],
			"OrderComponents": [
				{
					"name": "offerer",
					"type": "address"
				},
				{
					"name": "zone",
					"type": "address"
				},
				{
					"name": "offer",
					"type": "OfferItem[]"
				},
				{
					"name": "consideration",
					"type": "ConsiderationItem[]"
				},
				{
					"name": "orderType",
					"type": "uint8"
				},
				{
					"name": "startTime",
					"type": "uint256"
				},
				{
					"name": "endTime",
					"type": "uint256"
				},
				{
					"name": "zoneHash",
					"type": "bytes32"
				},
				{
					"name": "salt",
					"type": "uint256"
				},
				{
					"name": "conduitKey",
					"type": "bytes32"
				},
				{
					"name": "counter",
					"type": "uint256"
				}
			]
		}
	}`

	typedData, err := ethcoder.TypedDataFromJSON(typedDataJson)
	require.NoError(t, err)
	require.NotNil(t, typedData)
}
