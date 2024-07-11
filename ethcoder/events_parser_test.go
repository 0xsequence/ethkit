package ethcoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEventArgs(t *testing.T) {
	cases := []struct {
		in         string
		expect     string
		numArgs    int
		numIndexed int
	}{
		{
			"bytes32 num,   address[] indexed from, uint256 num,   (   address op,   (uint256 val, bytes32 data)) indexed yes,   address,  (int128 a, int64 b), uint256",
			"bytes32,address[],uint256,(address,(uint256,bytes32)),address,(int128,int64),uint256",
			7,
			2,
		},
		{ // its not actually valid selector, but use it for parser testing
			"bytes indexed blah, uint256[2][] yes, ( address yo, uint256[2] yes )[2][] indexed okay, address yes",
			"bytes,uint256[2][],(address,uint256[2])[2][],address",
			4,
			2,
		},
		{ // its not actually valid selector, but use it for parser testing
			"address from, (  uint256 num, address cool, (  address op, uint256 val )[2] hmm)[][] lol, uint256 val",
			"address,(uint256,address,(address,uint256)[2])[][],uint256",
			3,
			0,
		},
		{
			"address indexed from, address indexed to, uint256 value",
			"address,address,uint256",
			3,
			2,
		},
		{
			"bytes32,address,address indexed yes,((uint32,uint32,uint32,address,address,bool,bytes,uint256,address,uint256,uint256,uint256,bytes32),address[],bytes[],address,bytes) indexed cool,address,uint256,address indexed last",
			"bytes32,address,address,((uint32,uint32,uint32,address,address,bool,bytes,uint256,address,uint256,uint256,uint256,bytes32),address[],bytes[],address,bytes),address,uint256,address",
			7,
			3,
		},
		//..
		{
			"(address,uint256,string,string) indexed okay",
			"(address,uint256,string,string)",
			1,
			1,
		},
		{
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[] indexed good,(uint8,address,uint256,uint256,address)[] indexed last",
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			6,
			2,
		},
		{
			"address,(uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,address,uint40,uint40)",
			"address,(uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,address,uint40,uint40)",
			2,
			0,
		},
		{
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			"bytes32,address,address,address,(uint8,address,uint256,uint256)[],(uint8,address,uint256,uint256,address)[]",
			6,
			0,
		},
	}

	for _, c := range cases {
		tree, err := parseEventArgs(c.in)
		require.NoError(t, err)
		// spew.Dump(tree)

		out, typs, indexed, err := groupEventSelectorTree(tree, true)
		require.NoError(t, err)
		require.Equal(t, c.expect, out)
		// spew.Dump(typs)
		// spew.Dump(indexed)

		require.Equal(t, c.numArgs, len(typs))
		require.Equal(t, c.numArgs, len(indexed))

		numIndexed := 0
		for _, i := range indexed {
			if i {
				numIndexed++
			}
		}
		require.Equal(t, c.numIndexed, numIndexed)

		// ok, err := ValidateEventSig(fmt.Sprintf("Test(%s)", out))
		// require.NoError(t, err)
		// require.True(t, ok)
	}
}
