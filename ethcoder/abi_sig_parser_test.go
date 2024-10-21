package ethcoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseABISignature(t *testing.T) {
	cases := []struct {
		in         string
		sig        string
		argIndexed []bool
		argNames   []string
	}{
		{
			"bytes32 num,   address[] indexed from, uint256 val,   (   address op,   (uint256 val, bytes32 data)) indexed yes,   address,  (int128 a, int64 b), uint256",
			"bytes32,address[],uint256,(address,(uint256,bytes32)),address,(int128,int64),uint256",
			[]bool{false, true, false, true, false, false, false},
			[]string{"num", "from", "val", "yes", "", "", ""},
		},
		{ // its not actually valid selector, but use it for parser testing
			"bytes indexed blah, uint256[2][] yes, ( address yo, uint256[2] no )[2][] indexed okay, address last",
			"bytes,uint256[2][],(address,uint256[2])[2][],address",
			[]bool{true, false, true, false},
			[]string{"blah", "yes", "okay", "last"},
		},
		{ // its not actually valid selector, but use it for parser testing
			"address from, (  uint256 num, address cool, (  address op, uint256 val )[2] hmm)[][] lol, uint256 val",
			"address,(uint256,address,(address,uint256)[2])[][],uint256",
			[]bool{false, false, false},
			[]string{"from", "lol", "val"},
		},
		{
			"address indexed from, address indexed to, uint256 value",
			"address,address,uint256",
			[]bool{true, true, false},
			[]string{"from", "to", "value"},
		},
		{
			"bytes32,address,address indexed yes,((uint32,uint32,uint32,address,address,bool,bytes,uint256,address,uint256,uint256,uint256,bytes32),address[],bytes[],address,bytes) indexed cool,address,uint256,address indexed last",
			"bytes32,address,address,((uint32,uint32,uint32,address,address,bool,bytes,uint256,address,uint256,uint256,uint256,bytes32),address[],bytes[],address,bytes),address,uint256,address",
			[]bool{false, false, true, true, false, false, true},
			[]string{"", "", "yes", "cool", "", "", "last"},
		},
		{
			"(address,uint256,string,string) indexed okay",
			"(address,uint256,string,string)",
			[]bool{true},
			[]string{"okay"},
		},
		{
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[] indexed good,(uint8,address,uint256,uint256,address)[] indexed last",
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			[]bool{false, false, false, false, true, true},
			[]string{"", "", "", "", "good", "last"},
		},
		{
			"address,(uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,address,uint40,uint40)",
			"address,(uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,address,uint40,uint40)",
			[]bool{false, false},
			[]string{"", ""},
		},
		{
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			[]bool{false, false, false, false, false, false},
			[]string{"", "", "", "", "", ""},
		},

		//
		// repeat, with no arg names
		//
		{
			"bytes32,   address[] indexed, uint256,   (   address op,   (uint256 val, bytes32 data)),   address,  (int128 a, int64 b), uint256",
			"bytes32,address[],uint256,(address,(uint256,bytes32)),address,(int128,int64),uint256",
			[]bool{false, true, false, false, false, false, false},
			[]string{"", "", "", "", "", "", ""},
		},
		{ // its not actually valid selector, but use it for parser testing
			"bytes, uint256[2][], ( address yo, uint256[2] no )[2][] indexed, address",
			"bytes,uint256[2][],(address,uint256[2])[2][],address",
			[]bool{false, false, true, false},
			[]string{"", "", "", ""},
		},
		{ // its not actually valid selector, but use it for parser testing
			"address, (  uint256 num, address cool, (  address op, uint256 val )[2] hmm)[][], uint256",
			"address,(uint256,address,(address,uint256)[2])[][],uint256",
			[]bool{false, false, false},
			[]string{"", "", ""},
		},
		{
			"address,address,uint256",
			"address,address,uint256",
			[]bool{false, false, false},
			[]string{"", "", ""},
		},
		{
			"bytes32,address,address,((uint32,uint32,uint32,address,address,bool,bytes,uint256,address,uint256,uint256,uint256,bytes32),address[],bytes[],address,bytes),address,uint256,address",
			"bytes32,address,address,((uint32,uint32,uint32,address,address,bool,bytes,uint256,address,uint256,uint256,uint256,bytes32),address[],bytes[],address,bytes),address,uint256,address",
			[]bool{false, false, false, false, false, false, false},
			[]string{"", "", "", "", "", "", ""},
		},
		{
			"(address,uint256,string,string)",
			"(address,uint256,string,string)",
			[]bool{false},
			[]string{""},
		},
		{
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[] indexed,(uint8,address,uint256,uint256,address)[]",
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			[]bool{false, false, false, false, true, false},
			[]string{"", "", "", "", "", ""},
		},
		{
			"address,(uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,address,uint40,uint40)",
			"address,(uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,address,uint40,uint40)",
			[]bool{false, false},
			[]string{"", ""},
		},
		{
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			[]bool{false, false, false, false, false, false},
			[]string{"", "", "", "", "", ""},
		},
	}

	for _, c := range cases {
		tree, err := parseABISignatureArgs(c.in, 0)
		require.NoError(t, err)
		// spew.Dump(tree)

		out, typs, indexed, names, err := groupABISignatureTree(tree, true)
		require.NoError(t, err)
		require.Equal(t, c.sig, out)
		// spew.Dump(typs)
		// spew.Dump(indexed)
		// spew.Dump(names)

		require.Equal(t, len(c.argNames), len(typs))
		require.Equal(t, len(c.argNames), len(indexed))
		require.Equal(t, len(c.argNames), len(names))
		require.Equal(t, c.argIndexed, indexed)
		require.Equal(t, c.argNames, names)

		// ok, err := ValidateEventSig(fmt.Sprintf("Test(%s)", out))
		// require.NoError(t, err)
		// require.True(t, ok)
	}
}
