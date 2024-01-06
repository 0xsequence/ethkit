package main

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func execBalanceCmd(args string) (string, error) {
	cmd := NewBalanceCmd()
	actual := new(bytes.Buffer)
	cmd.SetOut(actual)
	cmd.SetErr(actual)
	cmd.SetArgs(strings.Split(args, " "))
	if err := cmd.Execute(); err != nil {
		return "", err
	}

	return actual.String(), nil
}

func Test_BalanceCmd(t *testing.T) {
	res, err := execBalanceCmd("0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742 --rpc-url https://nodes.sequence.app/sepolia")
	assert.Nil(t, err)
	assert.NotNil(t, res)
}

func Test_BalanceCmd_ValidWei(t *testing.T) {
	res, err := execBalanceCmd("0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742 --rpc-url https://nodes.sequence.app/sepolia")
	assert.Nil(t, err)
	assert.Equal(t, res, fmt.Sprintln(strconv.Itoa(500_000_000_000_000_000), "wei"))
}

func Test_BalanceCmd_ValidEther(t *testing.T) {
	res, err := execBalanceCmd("0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742 --rpc-url https://nodes.sequence.app/sepolia --ether")
	assert.Nil(t, err)
	assert.Equal(t, res, fmt.Sprintln(strconv.FormatFloat(0.5, 'f', -1, 64), "ether"))
}

func Test_BalanceCmd_InvalidAddress(t *testing.T) {
	res, err := execBalanceCmd("0x1 --rpc-url https://nodes.sequence.app/sepolia")
	assert.NotNil(t, err)
	assert.Empty(t, res)
	assert.Contains(t, err.Error(), "please provide a valid account address")
}

func Test_BalanceCmd_InvalidRPC(t *testing.T) {
	res, err := execBalanceCmd("0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742 --rpc-url nodes.sequence.app/sepolia")
	assert.NotNil(t, err)
	assert.Empty(t, res)
	assert.Contains(t, err.Error(), "please provide a valid rpc url")
}

func Test_BalanceCmd_NotExistingBlockHeigh(t *testing.T) {
	res, err := execBalanceCmd("0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742 --rpc-url https://nodes.sequence.app/sepolia --block " + fmt.Sprint(math.MaxInt64))
	assert.NotNil(t, err)
	assert.Empty(t, res)
	assert.Contains(t, err.Error(), "jsonrpc error -32000: header not found")
}

func Test_BalanceCmd_NotAValidStringBlockHeigh(t *testing.T) {
	res, err := execBalanceCmd("0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742 --rpc-url https://nodes.sequence.app/sepolia --block something")
	assert.NotNil(t, err)
	assert.Empty(t, res)
	assert.Contains(t, err.Error(), "invalid block height")
}

func Test_BalanceCmd_NotAValidNumberBlockHeigh(t *testing.T) {
	res, err := execBalanceCmd("0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742 --rpc-url https://nodes.sequence.app/sepolia --block -100")
	assert.NotNil(t, err)
	assert.Empty(t, res)
}
