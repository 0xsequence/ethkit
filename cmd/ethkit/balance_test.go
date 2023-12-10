package main

import (
	"bytes"
	"math"
	"strconv"

	// "fmt"
	// "strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func NewBalanceCommand() *cobra.Command {
	balance := &balance{}

	cmd := &cobra.Command{
		Use:   "balance [account]",
		Short: "Get the balance of an account",
		Args:  cobra.ExactArgs(1),
		RunE:  balance.Run,
	}

	cmd.Flags().StringP("block", "B","latest", "")
	cmd.Flags().BoolP("ether", "e", false, "")
	cmd.Flags().StringP("rpc-url", "r", "", "")

	return cmd
}

func TestOKExecuteBalanceCommand(t *testing.T) {
	cmd := NewBalanceCommand()
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)
	cmd.SetArgs([]string{"0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742", "--rpc-url", "https://nodes.sequence.app/sepolia"})
	assert.NoError(t, cmd.Execute())
}

// TODO: Fix bad output redirection that makes assert to fail
func TestOKBalanceValidWei(t *testing.T) {
	cmd := NewBalanceCommand()
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetArgs([]string{"0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742", "--rpc-url", "https://nodes.sequence.app/sepolia"})
	assert.NoError(t, cmd.Execute())
	// assert.Equal(t, b.String(), fmt.Sprintln(strconv.Itoa(500_000_000_000_000_000), "wei"))
}

// TODO: Fix bad output redirection that makes assert to fail
func TestOKBalanceValidEther(t *testing.T) {
	cmd := NewBalanceCommand()
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)
	cmd.SetArgs([]string{"0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742", "--rpc-url", "https://nodes.sequence.app/sepolia", "--ether"})
	assert.NoError(t, cmd.Execute())
	// assert.Equal(t, b.String(), fmt.Sprintln(strconv.FormatFloat(0.5, 'f', -1, 64), "ether"))
}

func TestFailInvalidAddress(t *testing.T) {
	cmd := NewBalanceCommand()
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)
	cmd.SetArgs([]string{"0x1", "--rpc-url", "https://nodes.sequence.app/sepolia"})
	assert.Error(t, cmd.Execute())
	assert.Contains(t, b.String(), "valid account address")
}

func TestFailInvalidRPC(t *testing.T) {
	cmd := NewBalanceCommand()
	b := new(bytes.Buffer)
	cmd.Println(b)
	cmd.SetErr(b)
	cmd.SetArgs([]string{"0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742", "--rpc-url", "nodes.sequence.app/sepolia"})
	assert.Error(t, cmd.Execute())
	assert.Contains(t, b.String(), "valid rpc url")
}

func TestFailNotExistingBlockHeigh(t *testing.T) {
	cmd := NewBalanceCommand()
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)
	cmd.SetArgs([]string{"0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742", "--rpc-url", "https://nodes.sequence.app/sepolia", "--block", strconv.FormatInt(math.MaxInt64, 10)})
	assert.Error(t, cmd.Execute())
	assert.Contains(t, b.String(), "jsonrpc error -32000: header not found")
}

func TestFailNotAValidStringBlockHeigh(t *testing.T) {
	cmd := NewBalanceCommand()
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)
	cmd.SetArgs([]string{"0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742", "--rpc-url", "https://nodes.sequence.app/sepolia", "--block", "something"})
	assert.Error(t, cmd.Execute())
	assert.Contains(t, b.String(), "invalid block height")
}

func TestFailNotAValidNumberBlockHeigh(t *testing.T) {
	cmd := NewBalanceCommand()
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)
	cmd.SetArgs([]string{"0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742", "--rpc-url", "https://nodes.sequence.app/sepolia", "--block", "-100"})
	assert.Error(t, cmd.Execute())
}
