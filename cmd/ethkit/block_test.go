package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/0xsequence/ethkit/go-ethereum/common/math"
	"github.com/stretchr/testify/assert"
)

func execBlockCmd(args string) (string, error) {
	cmd := NewBlockCmd()
	actual := new(bytes.Buffer)
	cmd.SetOut(actual)
	cmd.SetErr(actual)
	cmd.SetArgs(strings.Split(args, " "))
	if err := cmd.Execute(); err != nil {
		return "", err
	}

	return actual.String(), nil
}

func Test_BlockCmd(t *testing.T) {
	res, err := execBlockCmd("18855325 --rpc-url https://nodes.sequence.app/mainnet")
	assert.Nil(t, err)
	assert.NotNil(t, res)
}

func Test_BlockCmd_InvalidRpcUrl(t *testing.T) {
	res, err := execBlockCmd("18855325 --rpc-url nodes.sequence.app/mainnet")
	assert.Contains(t, err.Error(), "please provide a valid rpc url")
	assert.Empty(t, res)
}

// Note: this test will eventually fail
func Test_BlockCmd_NotFound(t *testing.T) {
	res, err := execBlockCmd(fmt.Sprint(math.MaxInt64) + " --rpc-url https://nodes.sequence.app/mainnet")
	assert.Contains(t, err.Error(), "not found")
	assert.Empty(t, res)
}

func Test_BlockCmd_InvalidBlockHeight(t *testing.T) {
	res, err := execBlockCmd("invalid --rpc-url https://nodes.sequence.app/mainnet")
	assert.Contains(t, err.Error(), "invalid block height")
	assert.Empty(t, res)
}

func Test_BlockCmd_HeaderValidJSON(t *testing.T) {
	res, err := execBlockCmd("18855325 --rpc-url https://nodes.sequence.app/mainnet --json")
	assert.Nil(t, err)
	h := Header{}
	var p Printable
	_ = p.FromStruct(h)
	for k := range p {
		assert.Contains(t, res, k)
	}
}

func Test_BlockCmd_BlockValidJSON(t *testing.T) {
	res, err := execBlockCmd("18855325 --rpc-url https://nodes.sequence.app/mainnet --full --json")
	assert.Nil(t, err)
	h := Block{}
	var p Printable
	_ = p.FromStruct(h)
	for k := range p {
		assert.Contains(t, res, k)
	}
}

func Test_BlockCmd_BlockValidFieldHash(t *testing.T) {
	// validating also that -f is case-insensitive
	res, err := execBlockCmd("18855325 --rpc-url https://nodes.sequence.app/mainnet --full -f HASh")
	assert.Nil(t, err)
	assert.Equal(t, res, "0x97e5c24dc2fd74f6e56773a0ad1cf29fe403130ca6ec1dd10ff8828d72b0a352\n")
}

func Test_BlockCmd_BlockInvalidField(t *testing.T) {
	res, err := execBlockCmd("18855325 --rpc-url https://nodes.sequence.app/mainnet --full -f invalid")
	assert.Nil(t, err)
	assert.Equal(t, res, "<nil>\n")
}
