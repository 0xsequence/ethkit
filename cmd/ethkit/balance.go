package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/params"
)

func init() {
	balance := &balance{}
	cmd := &cobra.Command{
		Use:   "balance [account]",
		Short: "Get the balance of an account",
		Args:  cobra.ExactArgs(1),
		RunE:  balance.Run,
	}

	cmd.Flags().StringP("block", "B", "latest", "The block height to query at")
	cmd.Flags().BoolP("ether", "e", false, "Format the balance in ether")
	cmd.Flags().StringP("rpc-url", "r", "", "The RPC endpoint to the blockchain node to interact with")

	rootCmd.AddCommand(cmd)
}

type balance struct {
}

func (c *balance) Run(cmd *cobra.Command, args []string) error {
	fAccount := cmd.Flags().Args()[0]
	fBlock, err := cmd.Flags().GetString("block")
	if err != nil {
		return err
	}
	fEther, err := cmd.Flags().GetBool("ether")
	if err != nil {
		return err
	}
	fRpc, err := cmd.Flags().GetString("rpc-url")
	if err != nil {
		return err
	}

	if !common.IsHexAddress(fAccount) {
		return errors.New("error: please provide a valid account address (e.g. 0x213a286A1AF3Ac010d4F2D66A52DeAf762dF7742)")
	}

	if _, err = url.ParseRequestURI(fRpc); err != nil {
		return errors.New("error: please provide a valid rpc url (e.g. https://nodes.sequence.app/mainnet)")
	}

	provider, err := ethrpc.NewProvider(fRpc)
	if err != nil {
		return err
	}

	block, err := strconv.ParseUint(fBlock, 10, 64)
	if err != nil {
		// TODO: implement support for all tags: earliest, latest, pending, finalized, safe
		if fBlock == "latest" {
			bh, err := provider.BlockNumber(context.Background())
			if err != nil {
				return err
			}
			block = bh
		} else {
			return errors.New("error: invalid block height")
		}
	}

	wei, err := provider.BalanceAt(context.Background(), common.HexToAddress(fAccount), big.NewInt(int64(block)))
	if err != nil {
		return err
	}

	if fEther {
		bal := weiToEther(wei)
		fmt.Println(bal, "ether")
	} else {
		fmt.Println(wei, "wei")
	}

	return nil
}

// https://github.com/ethereum/go-ethereum/issues/21221
func weiToEther(wei *big.Int) *big.Float {
	f := new(big.Float)
	f.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	f.SetMode(big.ToNearestEven)
	fWei := new(big.Float)
	fWei.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	fWei.SetMode(big.ToNearestEven)

	return f.Quo(fWei.SetInt(wei), big.NewFloat(params.Ether))
}
