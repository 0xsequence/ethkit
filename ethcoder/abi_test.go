package ethcoder

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
)

func TestAbiEncoding(t *testing.T) {
	cases := []struct {
		argTypes []string
		expected string
		input    []interface{}
	}{
		{
			argTypes: []string{
				"uint256[]",
				"uint256[]",
			},
			expected: `0x000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000800000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000002c00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000016`,
			input: []interface{}{
				[]*big.Int{big.NewInt(44)},
				[]*big.Int{big.NewInt(22)},
			},
		},
	}

	for _, i := range cases {
		packed, err := AbiCoder(i.argTypes, i.input)
		assert.NoError(t, err)

		// the expected value is the same
		assert.Equal(t, i.expected, hexutil.Encode(packed))

		// decode the value
		output := make([]interface{}, len(i.argTypes))
		err = AbiDecoder(i.argTypes, packed, output)
		assert.NoError(t, err)

		if !reflect.DeepEqual(output, i.input) {
			t.Fatal("encode/decode do not match")
		}
	}
}

func TestAbiDecoder(t *testing.T) {
	{
		input, err := HexDecode("0x000000000000000000000000000000000000000000007998f984c2040a5a9e01000000000000000000000000000000000000000000007998f984c2040a5a9e01")
		assert.NoError(t, err)
		var num, num2 *big.Int
		err = AbiDecoder([]string{"uint256", "uint256"}, input, []interface{}{&num, &num2})
		assert.NoError(t, err)
	}

	{
		input, err := HexDecode("0x000000000000000000000000000000000000000000007998f984c2040a5a9e01")
		assert.NoError(t, err)
		var num *big.Int
		err = AbiDecoder([]string{"uint256"}, input, []interface{}{&num})
		assert.NoError(t, err)
	}

	{
		input, err := HexDecode("0x000000000000000000000000000000000000000000007998f984c2040a5a9e01")
		assert.NoError(t, err)

		values, err := AbiDecoderWithReturnedValues([]string{"uint256"}, input)
		assert.NoError(t, err)
		assert.Len(t, values, 1)

		num, ok := values[0].(*big.Int)
		assert.True(t, ok)
		assert.Equal(t, "574228229235365901934081", num.String())
	}
}

func TestParseMethodABI(t *testing.T) {
	// correct usage
	{
		mabi, methodName, err := ParseMethodABI("balanceOf(address,uint256)", "uint256")
		assert.NoError(t, err)
		assert.Equal(t, "balanceOf", methodName)

		ownerAddress := common.HexToAddress("0x6615e4e985bf0d137196897dfa182dbd7127f54f")
		data, err := mabi.Pack("balanceOf", ownerAddress, big.NewInt(2))
		assert.NoError(t, err)

		assert.Equal(t, "0x00fdd58e0000000000000000000000006615e4e985bf0d137196897dfa182dbd7127f54f0000000000000000000000000000000000000000000000000000000000000002", HexEncode(data))
	}

	// correct usage
	{
		_, _, err := ParseMethodABI("someMethod(address)", "(uint256, bytes)")
		assert.NoError(t, err)

		// we also allow names for input/output arguments
		_, _, err = ParseMethodABI("someMethod(address owner)", "(uint256 count, bytes value)")
		assert.NoError(t, err)
	}

	// invalid usage
	{
		_, _, err := ParseMethodABI("balanceOf address, uint256)", "uint256")
		assert.Error(t, err)

		_, _, err = ParseMethodABI("balanceOf(address, uint256)", "blah")
		assert.Contains(t, "unsupported arg type: blah", err.Error())
	}
}

func TestAbiEncodeMethodCalldata(t *testing.T) {
	ownerAddress := common.HexToAddress("0x6615e4e985bf0d137196897dfa182dbd7127f54f")

	{
		calldata, err := AbiEncodeMethodCalldata("balanceOf(address,uint256)", []interface{}{ownerAddress, big.NewInt(2)})
		assert.NoError(t, err)
		assert.Equal(t, "0x00fdd58e0000000000000000000000006615e4e985bf0d137196897dfa182dbd7127f54f0000000000000000000000000000000000000000000000000000000000000002", HexEncode(calldata))

		// arrays
		calldata, err = AbiEncodeMethodCalldata("getCurrencyReserves(uint256[])", []interface{}{[]*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}})
		assert.NoError(t, err)
		assert.Equal(t, "0x209b96c500000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003", HexEncode(calldata))
	}

	{
		calldata, err := AbiEncodeMethodCalldataFromStringValues("balanceOf(address,uint256)", []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "2"})
		assert.NoError(t, err)
		assert.Equal(t, "0x00fdd58e0000000000000000000000006615e4e985bf0d137196897dfa182dbd7127f54f0000000000000000000000000000000000000000000000000000000000000002", HexEncode(calldata)) // same as above

		// arrays
		calldata, err = AbiEncodeMethodCalldataFromStringValues("getCurrencyReserves(uint256[])", []string{`["1","2","3"]`})
		assert.NoError(t, err)
		assert.Equal(t, "0x209b96c500000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003", HexEncode(calldata)) // same as above
	}
}

func TestAbiDecodeExpr(t *testing.T) {
	ret := "0x000000000000000000000000000000000000000000007998f984c2040a5a9e01"

	{
		retTrimmed, err := HexTrimLeadingZeros(ret)
		assert.NoError(t, err)
		assert.Equal(t, "0x7998f984c2040a5a9e01", retTrimmed)

		val, err := hexutil.DecodeBig(retTrimmed)
		assert.NoError(t, err)
		assert.Equal(t, "574228229235365901934081", val.String())
	}

	{
		input, err := HexDecode(ret)
		assert.NoError(t, err)

		var num *big.Int
		output := []interface{}{&num}

		err = AbiDecodeExpr("uint256", input, output)
		assert.NoError(t, err)
		assert.Equal(t, "574228229235365901934081", num.String())
	}
}

func TestAbiDecodeExprAndStringify(t *testing.T) {
	{
		values, err := AbiDecodeExprAndStringify("uint256", MustHexDecode("0x000000000000000000000000000000000000000000007998f984c2040a5a9e01"))
		assert.NoError(t, err)
		assert.Len(t, values, 1)
		assert.Equal(t, "574228229235365901934081", values[0])
	}

	{
		data, err := AbiCoder([]string{"uint256", "address"}, []interface{}{big.NewInt(1337), common.HexToAddress("0x6615e4e985bf0d137196897dfa182dbd7127f54f")})
		assert.NoError(t, err)

		values, err := AbiDecodeExprAndStringify("(uint256,address)", data)
		assert.NoError(t, err)
		assert.Len(t, values, 2)
		assert.Equal(t, "1337", values[0])
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", values[1])
	}

	{
		data, err := AbiCoder([]string{"bool", "bool"}, []interface{}{true, false})
		assert.NoError(t, err)

		values, err := AbiDecodeExprAndStringify("(bool,bool)", data)
		assert.NoError(t, err)
		assert.Len(t, values, 2)
		assert.Equal(t, "true", values[0])
		assert.Equal(t, "false", values[1])
	}

	{
		data, err := AbiCoder([]string{"bytes"}, []interface{}{[]byte{1, 2, 3, 4}})
		assert.NoError(t, err)

		values, err := AbiDecodeExprAndStringify("(bytes)", data)
		assert.NoError(t, err)
		assert.Len(t, values, 1)
		assert.Equal(t, "[1 2 3 4]", values[0])
	}
}

func TestAbiUnmarshalStringValues(t *testing.T) {
	{
		values, err := AbiUnmarshalStringValues([]string{"address", "uint256"}, []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "2"})
		assert.NoError(t, err)
		assert.Len(t, values, 2)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].(*big.Int)
		assert.True(t, ok)
		assert.Equal(t, int64(2), v2.Int64())
	}

	{
		values, err := AbiUnmarshalStringValues([]string{"address", "bytes8"}, []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0xaabbccddaabbccdd"})
		assert.NoError(t, err)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].([]uint8)
		assert.True(t, ok)
		assert.Equal(t, []uint8{170, 187, 204, 221, 170, 187, 204, 221}, v2)
	}

	{
		values, err := AbiUnmarshalStringValues([]string{"address", "bytes7"}, []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0xaabbccddaabbcc"})
		assert.NoError(t, err)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].([]uint8)
		assert.True(t, ok)
		assert.Equal(t, []uint8{170, 187, 204, 221, 170, 187, 204}, v2)
	}
}

// func TestAbiContractCall1(t *testing.T) {
// 	calldata, err := AbiEncodeMethodCalldata("getCurrencyReserves(uint256[])", []interface{}{[]*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}})
// 	assert.NoError(t, err)
// 	assert.Equal(t, "0x209b96c500000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003", HexEncode(calldata))

// 	ca := common.HexToAddress("0xa519711c25a631e55a6eac19d1f2858c97a86a95")
// 	txMsg := ethereum.CallMsg{
// 		To:   &ca,
// 		Data: calldata,
// 	}

// 	p, _ := ethrpc.NewProvider("https://rinkeby.infura.io/v3/xxxx")
// 	contractCallOutput, err := p.CallContract(context.Background(), txMsg, nil)
// 	assert.NoError(t, err)

// 	spew.Dump(contractCallOutput)

// 	var values []*big.Int
// 	err = AbiDecodeExpr("uint256[]", contractCallOutput, []interface{}{&values})
// 	assert.NoError(t, err)

// 	// spew.Dump(values)
// }

// func TestAbiContractCall2(t *testing.T) {
// 	calldata, err := AbiEncodeMethodCalldataFromStringValues("getCurrencyReserves(uint256[])", []string{`["1","2","3"]`})
// 	assert.NoError(t, err)
// 	assert.Equal(t, "0x209b96c500000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003", HexEncode(calldata))

// 	ca := common.HexToAddress("0xa519711c25a631e55a6eac19d1f2858c97a86a95")
// 	txMsg := ethereum.CallMsg{
// 		To:   &ca,
// 		Data: calldata,
// 	}

// 	p, _ := ethrpc.NewProvider("https://rinkeby.infura.io/v3/xxxx")
// 	contractCallOutput, err := p.CallContract(context.Background(), txMsg, nil)
// 	assert.NoError(t, err)

// 	spew.Dump(contractCallOutput)

// 	values, err := AbiDecodeExprAndStringify("uint256[]", contractCallOutput)
// 	assert.NoError(t, err)

// 	spew.Dump(values)
// }
