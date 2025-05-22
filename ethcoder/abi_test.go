package ethcoder

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/bytedance/sonic"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestABIPackArguments(t *testing.T) {
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
		packed, err := ABIPackArguments(i.argTypes, i.input)
		assert.NoError(t, err)

		// the expected value is the same
		assert.Equal(t, i.expected, hexutil.Encode(packed))

		// decode the value
		output := make([]interface{}, len(i.argTypes))
		err = ABIUnpackArgumentsByRef(i.argTypes, packed, output)
		assert.NoError(t, err)

		if !reflect.DeepEqual(output, i.input) {
			t.Fatal("encode/decode do not match")
		}
	}
}

func TestABIUnpackArguments(t *testing.T) {
	{
		input, err := HexDecode("0x000000000000000000000000000000000000000000007998f984c2040a5a9e01000000000000000000000000000000000000000000007998f984c2040a5a9e01")
		assert.NoError(t, err)
		var num, num2 *big.Int
		err = ABIUnpackArgumentsByRef([]string{"uint256", "uint256"}, input, []interface{}{&num, &num2})
		assert.NoError(t, err)
	}

	{
		input, err := HexDecode("0x000000000000000000000000000000000000000000007998f984c2040a5a9e01")
		assert.NoError(t, err)
		var num *big.Int
		err = ABIUnpackArgumentsByRef([]string{"uint256"}, input, []interface{}{&num})
		assert.NoError(t, err)
	}

	{
		input, err := HexDecode("0x000000000000000000000000000000000000000000007998f984c2040a5a9e01")
		assert.NoError(t, err)

		values, err := ABIUnpackArguments([]string{"uint256"}, input)
		assert.NoError(t, err)
		assert.Len(t, values, 1)

		num, ok := values[0].(*big.Int)
		assert.True(t, ok)
		assert.Equal(t, "574228229235365901934081", num.String())
	}
}

// func TestParseMethodABI(t *testing.T) {
// 	// correct usage
// 	{
// 		mabi, methodName, err := ParseMethodABI("balanceOf(address,uint256)", "uint256")
// 		assert.NoError(t, err)
// 		assert.Equal(t, "balanceOf", methodName)

// 		ownerAddress := common.HexToAddress("0x6615e4e985bf0d137196897dfa182dbd7127f54f")
// 		data, err := mabi.Pack("balanceOf", ownerAddress, big.NewInt(2))
// 		assert.NoError(t, err)

// 		assert.Equal(t, "0x00fdd58e0000000000000000000000006615e4e985bf0d137196897dfa182dbd7127f54f0000000000000000000000000000000000000000000000000000000000000002", HexEncode(data))
// 	}

// 	// correct usage
// 	{
// 		_, _, err := ParseMethodABI("someMethod(address)", "(uint256, bytes)")
// 		assert.NoError(t, err)

// 		// we also allow names for input/output arguments
// 		_, _, err = ParseMethodABI("someMethod(address owner)", "(uint256 count, bytes value)")
// 		assert.NoError(t, err)

// 		// no args
// 		_, _, err = ParseMethodABI("read()", "uint256")
// 		assert.NoError(t, err)
// 	}

// 	// invalid usage
// 	{
// 		_, _, err := ParseMethodABI("balanceOf address, uint256)", "uint256")
// 		assert.Error(t, err)

// 		_, _, err = ParseMethodABI("balanceOf(address, uint256)", "blah")
// 		assert.Contains(t, "unsupported arg type: blah", err.Error())
// 	}
// }

func TestABIEncodeMethodCalldata(t *testing.T) {
	ownerAddress := common.HexToAddress("0x6615e4e985bf0d137196897dfa182dbd7127f54f")

	{
		calldata, err := ABIEncodeMethodCalldata("balanceOf(address,uint256)", []interface{}{ownerAddress, big.NewInt(2)})
		assert.NoError(t, err)
		assert.Equal(t, "0x00fdd58e0000000000000000000000006615e4e985bf0d137196897dfa182dbd7127f54f0000000000000000000000000000000000000000000000000000000000000002", HexEncode(calldata))

		// arrays
		calldata, err = ABIEncodeMethodCalldata("getCurrencyReserves(uint256[])", []interface{}{[]*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}})
		assert.NoError(t, err)
		assert.Equal(t, "0x209b96c500000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003", HexEncode(calldata))
	}

	{
		calldata, err := ABIEncodeMethodCalldataFromStringValues("balanceOf(address,uint256)", []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "2"})
		assert.NoError(t, err)
		assert.Equal(t, "0x00fdd58e0000000000000000000000006615e4e985bf0d137196897dfa182dbd7127f54f0000000000000000000000000000000000000000000000000000000000000002", HexEncode(calldata)) // same as above

		// arrays
		calldata, err = ABIEncodeMethodCalldataFromStringValues("getCurrencyReserves(uint256[])", []string{`["1","2","3"]`})
		assert.NoError(t, err)
		assert.Equal(t, "0x209b96c500000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003", HexEncode(calldata)) // same as above
	}
}

func TestABIDecodeExpr(t *testing.T) {
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

		err = ABIUnpackArgumentsByRef([]string{"uint256"}, input, output)
		assert.NoError(t, err)
		assert.Equal(t, "574228229235365901934081", num.String())
	}
}

func TestABIDecodeExprAndStringify(t *testing.T) {
	{
		values, err := ABIUnpackAndStringify("uint256", MustHexDecode("0x000000000000000000000000000000000000000000007998f984c2040a5a9e01"))
		assert.NoError(t, err)
		assert.Len(t, values, 1)
		assert.Equal(t, "574228229235365901934081", values[0])
	}

	{
		data, err := ABIPackArguments([]string{"uint256", "address"}, []interface{}{big.NewInt(1337), common.HexToAddress("0x6615e4e985bf0d137196897dfa182dbd7127f54f")})
		assert.NoError(t, err)

		values, err := ABIUnpackAndStringify("(uint256,address)", data)
		assert.NoError(t, err)
		assert.Len(t, values, 2)
		assert.Equal(t, "1337", values[0])
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", values[1])
	}

	{
		data, err := ABIPackArguments([]string{"bool", "bool"}, []interface{}{true, false})
		assert.NoError(t, err)

		values, err := ABIUnpackAndStringify("(bool,bool)", data)
		assert.NoError(t, err)
		assert.Len(t, values, 2)
		assert.Equal(t, "true", values[0])
		assert.Equal(t, "false", values[1])
	}

	{
		data, err := ABIPackArguments([]string{"bytes"}, []interface{}{[]byte{1, 2, 3, 4}})
		assert.NoError(t, err)

		values, err := ABIUnpackAndStringify("(bytes)", data)
		assert.NoError(t, err)
		assert.Len(t, values, 1)
		assert.Equal(t, "[1 2 3 4]", values[0])
	}
}

func TestABIUnmarshalStringValuesAny(t *testing.T) {
	{
		values, err := ABIUnmarshalStringValuesAny([]string{"address", "uint256"}, []any{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "2"})
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
		values, err := ABIUnmarshalStringValuesAny([]string{"address", "uint256"}, []any{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0x123456"})
		assert.NoError(t, err)
		assert.Len(t, values, 2)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].(*big.Int)
		assert.True(t, ok)
		assert.Equal(t, int64(1193046), v2.Int64())
	}

	{
		values, err := ABIUnmarshalStringValuesAny([]string{"address", "bytes8"}, []any{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0xaabbccddaabbccdd"})
		assert.NoError(t, err)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].([8]byte)
		assert.True(t, ok)
		assert.Equal(t, [8]byte{170, 187, 204, 221, 170, 187, 204, 221}, v2)
	}

	{
		values, err := ABIUnmarshalStringValuesAny([]string{"address", "bytes7"}, []any{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0xaabbccddaabbcc"})
		assert.NoError(t, err)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].([7]byte)
		assert.True(t, ok)
		assert.Equal(t, [7]byte{170, 187, 204, 221, 170, 187, 204}, v2)
	}

	{
		values, err := ABIUnmarshalStringValuesAny([]string{"address", "uint256"}, []any{"", "2"})
		assert.Error(t, err)
		assert.Len(t, values, 0)
	}

	{
		values, err := ABIUnmarshalStringValuesAny([]string{"bytes", "uint256"}, []any{"0", "2"})
		assert.Error(t, err)
		assert.Len(t, values, 0)
	}

	{
		values, err := ABIUnmarshalStringValuesAny([]string{"bytes", "uint256"}, []any{"0z", "2"})
		assert.Error(t, err)
		assert.Len(t, values, 0)
	}

	{
		values, err := ABIUnmarshalStringValuesAny([]string{"address", "uint256"}, []any{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "2"})
		require.NoError(t, err)
		require.Len(t, values, 2)

		v1, ok := values[0].(common.Address)
		require.True(t, ok)
		require.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())
	}

	{
		in := []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0x1231f65f29f98e7D71A4655cCD7B2bc441211feb"}
		values, err := ABIUnmarshalStringValuesAny([]string{"address[]"}, []any{in})
		require.NoError(t, err)

		require.Len(t, values, 1)
		require.Len(t, values[0], 2)

		a1, ok := values[0].([]common.Address)
		require.True(t, ok)

		require.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", a1[0].String())
		require.Equal(t, "0x1231F65F29F98E7d71a4655CCD7B2bC441211FeB", a1[1].String())
	}

	{
		in := []string{"1234", "0x1231f65f29f98e7D71A4655cCD7B2bc441211feb"}
		values, err := ABIUnmarshalStringValuesAny([]string{"(uint256,address)"}, []any{in})
		require.NoError(t, err)

		require.Len(t, values, 1)
		require.Len(t, values[0], 2)

		a1, ok := values[0].([]any)
		require.True(t, ok)

		a1a, ok := a1[0].(*big.Int)
		require.True(t, ok)

		a1b, ok := a1[1].(common.Address)
		require.True(t, ok)

		require.Equal(t, "1234", a1a.String())
		require.Equal(t, "0x1231F65F29F98E7d71a4655CCD7B2bC441211FeB", a1b.String())
	}

	{
		// (uint256,(uint256,address[]))
		in := []any{"444", []any{"1234", []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0x1231f65f29f98e7D71A4655cCD7B2bc441211feb"}}}
		values, err := ABIUnmarshalStringValuesAny([]string{"uint256", "(uint256,address[])"}, in)
		require.NoError(t, err)

		require.Len(t, values, 2)
		require.Len(t, values[1], 2)

		a1, ok := values[0].(*big.Int)
		require.True(t, ok)
		require.Equal(t, "444", a1.String())

		a2, ok := values[1].([]any)
		require.True(t, ok)
		require.Len(t, a2, 2)

		a2a, ok := a2[0].(*big.Int)
		require.True(t, ok)
		require.Equal(t, "1234", a2a.String())

		a2b, ok := a2[1].([]common.Address)
		require.True(t, ok)
		require.Len(t, a2b, 2)
		require.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", a2b[0].String())
		require.Equal(t, "0x1231F65F29F98E7d71a4655CCD7B2bC441211FeB", a2b[1].String())
	}
}

func TestABIUnmarshalStringValues(t *testing.T) {
	{
		values, err := ABIUnmarshalStringValues([]string{"address", "uint256"}, []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "2"})
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
		values, err := ABIUnmarshalStringValues([]string{"address", "uint256"}, []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0x123456"})
		assert.NoError(t, err)
		assert.Len(t, values, 2)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].(*big.Int)
		assert.True(t, ok)
		assert.Equal(t, int64(1193046), v2.Int64())
	}

	{
		values, err := ABIUnmarshalStringValues([]string{"address", "bytes8"}, []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0xaabbccddaabbccdd"})
		assert.NoError(t, err)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].([8]byte)
		assert.True(t, ok)
		assert.Equal(t, [8]byte{170, 187, 204, 221, 170, 187, 204, 221}, v2)
	}

	{
		values, err := ABIUnmarshalStringValues([]string{"address", "bytes7"}, []string{"0x6615e4e985bf0d137196897dfa182dbd7127f54f", "0xaabbccddaabbcc"})
		assert.NoError(t, err)

		v1, ok := values[0].(common.Address)
		assert.True(t, ok)
		assert.Equal(t, "0x6615e4e985BF0D137196897Dfa182dBD7127f54f", v1.String())

		v2, ok := values[1].([7]byte)
		assert.True(t, ok)
		assert.Equal(t, [7]byte{170, 187, 204, 221, 170, 187, 204}, v2)
	}

	{
		values, err := ABIUnmarshalStringValues([]string{"address", "uint256"}, []string{"", "2"})
		assert.Error(t, err)
		assert.Len(t, values, 0)
	}

	{
		values, err := ABIUnmarshalStringValues([]string{"bytes", "uint256"}, []string{"0", "2"})
		assert.Error(t, err)
		assert.Len(t, values, 0)
	}

	{
		values, err := ABIUnmarshalStringValues([]string{"bytes", "uint256"}, []string{"0z", "2"})
		assert.Error(t, err)
		assert.Len(t, values, 0)
	}

	{
		values, err := ABIUnmarshalStringValues([]string{"uint256[]"}, []string{`["1","2","3"]`})
		assert.NoError(t, err)

		// nested by type list, ie. "uint256[]" is a single argument of an array type
		assert.Len(t, values, 1)
		assert.Len(t, values[0], 3)
	}

	{
		values, err := ABIUnmarshalStringValues([]string{"uint256[4]"}, []string{`["1","2","3","4"]`})
		assert.NoError(t, err)

		// nested by type list, ie. "uint256[]" is a single argument of an array type
		assert.Len(t, values, 1)
		assert.Len(t, values[0], 4)
	}
}

// func TestABIContractCall1(t *testing.T) {
// 	calldata, err := ABIEncodeMethodCalldata("getCurrencyReserves(uint256[])", []interface{}{[]*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}})
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

// func TestABIContractCall2(t *testing.T) {
// 	calldata, err := ABIEncodeMethodCalldataFromStringValues("getCurrencyReserves(uint256[])", []string{`["1","2","3"]`})
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

// 	values, err := ABIUnpackAndStringify("uint256[]", contractCallOutput)
// 	assert.NoError(t, err)

// 	spew.Dump(values)
// }

func TestEncodeContractCall(t *testing.T) {
	t.Run("simple transferFrom, not named", func(t *testing.T) {
		// Encode simple transferFrom, not named
		res, err := EncodeContractCall(ContractCallDef{
			ABI:  `[{"name":"transferFrom","type":"function","inputs":[{"name":"_from","type":"address"},{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}]}]`,
			Func: "transferFrom",
			Args: []any{"0x0dc9603d4da53841C1C83f3B550C6143e60e0425", "0x0dc9603d4da53841C1C83f3B550C6143e60e0425", "100"},
		})
		require.Nil(t, err)
		require.Equal(t, "0x23b872dd0000000000000000000000000dc9603d4da53841c1c83f3b550c6143e60e04250000000000000000000000000dc9603d4da53841c1c83f3b550c6143e60e04250000000000000000000000000000000000000000000000000000000000000064", res)
	})

	t.Run("simple transferFrom, selector", func(t *testing.T) {
		// Encode simple transferFrom, selector
		res, err := EncodeContractCall(ContractCallDef{
			ABI:  `transferFrom(address,address,uint256)`,
			Args: []any{"0x0dc9603d4da53841C1C83f3B550C6143e60e0425", "0x0dc9603d4da53841C1C83f3B550C6143e60e0425", "100"},
		})
		require.Nil(t, err)
		require.Equal(t, "0x23b872dd0000000000000000000000000dc9603d4da53841c1c83f3b550c6143e60e04250000000000000000000000000dc9603d4da53841c1c83f3b550c6143e60e04250000000000000000000000000000000000000000000000000000000000000064", res)
	})

	// Encode simple transferFrom, named
	// res, err = EncodeContractCall(ContractCallDef{
	// 	ABI:  `[{"name":"transferFrom","type":"function","inputs":[{"name":"_from","type":"address"},{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}]}]`,
	// 	Func: "transferFrom",
	// 	Args: json.RawMessage(`{"_from": "0x0dc9603d4da53841C1C83f3B550C6143e60e0425", "_value": "100", "_to": "0x0dc9603d4da53841C1C83f3B550C6143e60e0425"}`),
	// })
	// require.Nil(t, err)
	// require.Equal(t, res, "0x23b872dd0000000000000000000000000dc9603d4da53841c1c83f3b550c6143e60e04250000000000000000000000000dc9603d4da53841c1c83f3b550c6143e60e04250000000000000000000000000000000000000000000000000000000000000064")

	t.Run("simple transferFrom, not named, passed as function", func(t *testing.T) {
		// Encode simple transferFrom, not named, passed as function
		res, err := EncodeContractCall(ContractCallDef{
			ABI:  `transferFrom(address,address,uint256)`,
			Args: []any{"0x13915b1ea28Fd2E8197c88ff9D2422182E83bf25", "0x4Ad47F1611c78C824Ff3892c4aE1CC04637D6462", "5192381927398174182391237"},
		})
		require.Nil(t, err)
		require.Equal(t, "0x23b872dd00000000000000000000000013915b1ea28fd2e8197c88ff9d2422182e83bf250000000000000000000000004ad47f1611c78c824ff3892c4ae1cc04637d6462000000000000000000000000000000000000000000044b87969b06250e50bdc5", res)
	})

	// Encode simple transferFrom, named, passed as function
	// res, err = EncodeContractCall(ContractCallDef{
	// 	ABI:  `transferFrom(address _from,address _to,uint256 _value)`,
	// 	Func: "transferFrom",
	// 	Args: json.RawMessage(`{"_from": "0x13915b1ea28Fd2E8197c88ff9D2422182E83bf25", "_value": "5192381927398174182391237", "_to": "0x4Ad47F1611c78C824Ff3892c4aE1CC04637D6462"}`),
	// })
	// require.Nil(t, err)
	// require.Equal(t, res, "0x23b872dd00000000000000000000000013915b1ea28fd2e8197c88ff9d2422182e83bf250000000000000000000000004ad47f1611c78c824ff3892c4ae1cc04637d6462000000000000000000000000000000000000000000044b87969b06250e50bdc5")

	// // Encode nested bytes, passed as function
	// nestedEncodeType1 := ContractCallDef{
	// 	ABI:  `transferFrom(uint256)`,
	// 	Args: []any{"481923749816926378123"},
	// }

	// nestedEncodeType2 := ContractCallDef{
	// 	ABI:  `hola(string)`,
	// 	Args: []any{"mundo"},
	// }

	// net1jsn, err := sonic.ConfigFastest.Marshal(nestedEncodeType1)
	// require.Nil(t, err)

	// net2jsn, err := sonic.ConfigFastest.Marshal(nestedEncodeType2)
	// require.Nil(t, err)

	t.Run("nested transferFrom, not named", func(t *testing.T) {
		arg2, err := ABIEncodeMethodCalldataFromStringValues("transferFrom(uint256)", []string{"481923749816926378123"})
		require.NoError(t, err)

		arg3, err := ABIEncodeMethodCalldataFromStringValues("hola(string)", []string{"mundo"})
		require.NoError(t, err)

		arg2Hex := "0x" + common.Bytes2Hex(arg2)
		arg3Hex := "0x" + common.Bytes2Hex(arg3)

		res, err := EncodeContractCall(ContractCallDef{
			ABI:  "caller(address,bytes,bytes)",
			Args: []any{"0x13915b1ea28Fd2E8197c88ff9D2422182E83bf25", arg2Hex, arg3Hex},
		})
		require.Nil(t, err)
		require.Equal(t, "0x8b6701df00000000000000000000000013915b1ea28fd2e8197c88ff9d2422182e83bf25000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000000002477a11f7e00000000000000000000000000000000000000000000001a2009191df61e988b0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000646ce8ea55000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000056d756e646f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", res)
	})

	// Fail passing named args to non-named abi
	// res, err = EncodeContractCall(ContractCallDef{
	// 	ABI:  `transferFrom(address,uint256)`,
	// 	Func: "transferFrom",
	// 	Args: json.RawMessage(`{"_from": "0x13915b1ea28Fd2E8197c88ff9D2422182E83bf25", "_value": "5192381927398174182391237", "_to": "0x4Ad47F1611c78C824Ff3892c4aE1CC04637D6462"}`),
	// })
	// assert.NotNil(t, err)

	t.Run("accept passing ordened args to named abi", func(t *testing.T) {
		// Accept passing ordened args to named abi
		res, err := EncodeContractCall(ContractCallDef{
			ABI:  `transferFrom(address _from,address _to,uint256 _value)`,
			Args: []any{"0x13915b1ea28Fd2E8197c88ff9D2422182E83bf25", "0x4Ad47F1611c78C824Ff3892c4aE1CC04637D6462", "9"},
		})
		require.Nil(t, err)
		require.Equal(t, "0x23b872dd00000000000000000000000013915b1ea28fd2e8197c88ff9d2422182e83bf250000000000000000000000004ad47f1611c78c824ff3892c4ae1cc04637d64620000000000000000000000000000000000000000000000000000000000000009", res)

		// ...
		res, err = EncodeContractCall(ContractCallDef{
			ABI: `fillOrder(uint256 orderId, uint256 maxCost, address[] fees, (uint256 a, address b) extra)`,
			Args: []any{
				"48774435471364917511246724398022004900255301025912680232738918790354204737320",
				"1000000000000000000",
				[]string{"0x8541D65829f98f7D71A4655cCD7B2bB8494673bF"},
				[]string{"123456789", "0x1231f65f29f98e7D71A4655cCD7B2bc441211feb"},
			},
		})
		require.Nil(t, err)
		require.Equal(t, "0x326a62086bd55a2877890bd58871eefe886770a7734077a74981910a75d7b1f044b5bf280000000000000000000000000000000000000000000000000de0b6b3a764000000000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000000000000000075bcd150000000000000000000000001231f65f29f98e7d71a4655ccd7b2bc441211feb00000000000000000000000000000000000000000000000000000000000000010000000000000000000000008541d65829f98f7d71a4655ccd7b2bb8494673bf", res)
	})

	// NOTE: currently unsupported, but definitely doable
	// t.Run("array of tuples", func(t *testing.T) {
	// 	// Accept passing ordened args to named abi
	// 	res, err := EncodeContractCall(ContractCallDef{
	// 		ABI: `test((address,uint256)[])`,
	// 		Args: []any{
	// 			[]any{
	// 				[]any{"0x13915b1ea28Fd2E8197c88ff9D2422182E83bf25", "1234"},
	// 				[]any{"0x56915b1ea28Fd2E8197c88ff9D2422182E83bf25", "5678"},
	// 			},
	// 		},
	// 	})
	// 	require.Nil(t, err)
	// 	require.Equal(t, "0xabbb", res)
	// })

	t.Run("nested types example", func(t *testing.T) {
		// Nested types example, also using JSON as input
		//
		// NOTE: please see go-sequence TestRecoverTransactionIntent for more examples, especially for nested types
		// and encoding.
		jsonContractCall := `{
			"abi": "fillOrKillOrder(uint256 orderId, uint256 maxCost, address[] fees, bytes data)",
			"args": [
				"48774435471364917511246724398022004900255301025912680232738918790354204737320",
				"1000000000000000000",
				["0x8541D65829f98f7D71A4655cCD7B2bB8494673bF"],
				{
					"abi": "notExpired(uint256,string)",
					"args": [
						"1600000000",
						"Nov 1st, 2020"
					]
				}
			]
		}`

		var contractCall ContractCallDef
		err := sonic.ConfigFastest.Unmarshal([]byte(jsonContractCall), &contractCall)
		require.NoError(t, err)

		res, err := EncodeContractCall(contractCall)
		require.Nil(t, err)
		require.Equal(t, "0x6365f1646bd55a2877890bd58871eefe886770a7734077a74981910a75d7b1f044b5bf280000000000000000000000000000000000000000000000000de0b6b3a7640000000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000c000000000000000000000000000000000000000000000000000000000000000010000000000000000000000008541d65829f98f7d71a4655ccd7b2bb8494673bf000000000000000000000000000000000000000000000000000000000000008446c421fa000000000000000000000000000000000000000000000000000000005f5e10000000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000000d4e6f76203173742c20323032300000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", res)
	})

	t.Run("bytes32", func(t *testing.T) {
		jsonContractCall := `{
			"abi": "test(bytes32 val)",
			"args": [
				"0x0000000000000000000000000000000000000000000000000000000000000000"
			]
		}`

		var contractCall ContractCallDef
		err := sonic.ConfigFastest.Unmarshal([]byte(jsonContractCall), &contractCall)
		require.NoError(t, err)

		res, err := EncodeContractCall(contractCall)
		require.Nil(t, err)
		require.Equal(t, "0x993723210000000000000000000000000000000000000000000000000000000000000000", res)
	})

	t.Run("bytes8", func(t *testing.T) {
		jsonContractCall := `{
			"abi": "test(bytes8 val)",
			"args": [
				"0x1234567890abcdef"
			]
		}`

		var contractCall ContractCallDef
		err := sonic.ConfigFastest.Unmarshal([]byte(jsonContractCall), &contractCall)
		require.NoError(t, err)

		res, err := EncodeContractCall(contractCall)
		require.Nil(t, err)
		require.Equal(t, "0x531a65f21234567890abcdef000000000000000000000000000000000000000000000000", res)
	})

	t.Run("numbers", func(t *testing.T) {
		input := [][]any{
			{"test(uint8 val)", uint8(123), "123", "0xf1820bdc000000000000000000000000000000000000000000000000000000000000007b"},
			{"test(uint16 val)", uint16(4567), "0x123", "0x1891002e0000000000000000000000000000000000000000000000000000000000000123"},
			{"test(uint32 val)", uint32(22136), "0x5678", "0xe3cff6340000000000000000000000000000000000000000000000000000000000005678"},
			{"test(uint64 val)", uint64(31337), "31337", "0xb0e0b9ed0000000000000000000000000000000000000000000000000000000000007a69"},
			{"test(uint256 val)", big.NewInt(34952), "0x8888", "0x29e99f070000000000000000000000000000000000000000000000000000000000008888"},
			{"test(uint val)", big.NewInt(34952), "0x8888", "0x29e99f070000000000000000000000000000000000000000000000000000000000008888"}, // alias for uint256
			{"test(int8 val)", uint8(123), "123", "0x2b58697e000000000000000000000000000000000000000000000000000000000000007b"},
			{"test(int16 val)", uint16(4567), "0x123", "0x87d12c1b0000000000000000000000000000000000000000000000000000000000000123"},
			{"test(int32 val)", uint32(22136), "0x5678", "0x7747cc750000000000000000000000000000000000000000000000000000000000005678"},
			{"test(int64 val)", uint64(31337), "31337", "0xb3a5eb290000000000000000000000000000000000000000000000000000000000007a69"},
			{"test(int256 val)", big.NewInt(34952), "0x8888", "0x9b22c05d0000000000000000000000000000000000000000000000000000000000008888"},
			{"test(int val)", big.NewInt(34952), "0x8888", "0x9b22c05d0000000000000000000000000000000000000000000000000000000000008888"}, // alias for int256
		}
		for _, in := range input {
			res, err := EncodeContractCall(ContractCallDef{
				ABI:  in[0].(string),
				Args: []any{in[2]},
			})
			require.NoError(t, err)
			require.Equal(t, in[3], res, "in: %v", in[0])
		}
	})
}

func TestABIEncodeTuple(t *testing.T) {
	t.Run("tuple", func(t *testing.T) {
		typ, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
			{Name: "x", Type: "address"},
			{Name: "y", Type: "uint256"},
		})
		require.NoError(t, err)

		type Tuple struct {
			X common.Address `abi:"x"`
			Y *big.Int       `abi:"y"`
		}

		args := abi.Arguments{abi.Argument{Type: typ}}

		in := Tuple{
			X: common.HexToAddress("0x1231f65f29f98e7D71A4655cCD7B2bc441211feb"),
			Y: big.NewInt(1600000000),
		}

		encoded, err := args.Pack(in)
		require.NoError(t, err)
		require.Equal(t, "0000000000000000000000001231f65f29f98e7d71a4655ccd7b2bc441211feb000000000000000000000000000000000000000000000000000000005f5e1000", common.Bytes2Hex(encoded))
	})

	t.Run("tuple[]", func(t *testing.T) {
		typ, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{
			{Name: "x", Type: "address"},
			{Name: "y", Type: "uint256"},
		})
		require.NoError(t, err)

		type Tuple struct {
			X common.Address `abi:"x"`
			Y *big.Int       `abi:"y"`
		}

		args := abi.Arguments{abi.Argument{Type: typ}}

		in := []Tuple{
			{X: common.HexToAddress("0x1231f65f29f98e7D71A4655cCD7B2bc441211feb"), Y: big.NewInt(1600000000)},
			{X: common.HexToAddress("0x5671f65f29f98e7D71A4655cCD7B2bc441211feb"), Y: big.NewInt(8888)},
		}

		encoded, err := args.Pack(in)
		require.NoError(t, err)
		require.Equal(t, "000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000020000000000000000000000001231f65f29f98e7d71a4655ccd7b2bc441211feb000000000000000000000000000000000000000000000000000000005f5e10000000000000000000000000005671f65f29f98e7d71a4655ccd7b2bc441211feb00000000000000000000000000000000000000000000000000000000000022b8", common.Bytes2Hex(encoded))
	})
}
