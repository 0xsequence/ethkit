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
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

const (
	flagBlockField = "field"
	flagBlockFull = "full"
	flagBlockRpcUrl = "rpc-url"
	flagBlockJson = "json"
)

func init() {
	rootCmd.AddCommand(NewBlockCmd())
}

type block struct {
}

// NewBlockCommand returns a new build command to retrieve a block.
func NewBlockCmd() *cobra.Command {
	c := &block{}
	cmd := &cobra.Command{
		Use:     "block [number|tag]",
		Short:   "Get the information about the block",
		Aliases: []string{"bl"},
		Args:    cobra.ExactArgs(1),
		RunE:    c.Run,
	}

	cmd.Flags().StringP(flagBlockField, "f", "", "Get the specific field of a block")
	cmd.Flags().Bool(flagBlockFull, false, "Get the full block information")
	cmd.Flags().StringP(flagBlockRpcUrl, "r", "", "The RPC endpoint to the blockchain node to interact with")
	cmd.Flags().BoolP(flagBlockJson, "j", false, "Print the block as JSON")

	return cmd
}

func (c *block) Run(cmd *cobra.Command, args []string) error {
	fBlock := cmd.Flags().Args()[0]
	fField, err := cmd.Flags().GetString(flagBlockField)
	if err != nil {
		return err
	}
	fFull, err := cmd.Flags().GetBool(flagBlockFull)
	if err != nil {
		return err
	}
	fRpc, err := cmd.Flags().GetString(flagBlockRpcUrl)
	if err != nil {
		return err
	}
	fJson, err := cmd.Flags().GetBool(flagBlockJson)
	if err != nil {
		return err
	}

	if _, err = url.ParseRequestURI(fRpc); err != nil {
		return errors.New("error: please provide a valid rpc url (e.g. https://nodes.sequence.app/mainnet)")
	}

	provider, err := ethrpc.NewProvider(fRpc)
	if err != nil {
		return err
	}

	bh, err := strconv.ParseUint(fBlock, 10, 64)
	if err != nil {
		// TODO: implement support for all tags: earliest, latest, pending, finalized, safe
		return errors.New("error: invalid block height")
	}

	block, err := provider.BlockByNumber(context.Background(), big.NewInt(int64(bh)))
	if err != nil {
		return err
	}

	var obj any
	obj = NewHeader(block)

	if fFull {
		obj = NewBlock(block)
	}

	if fField != "" {
		obj = GetValueByJSONTag(obj, fField)
	}

	if fJson {
		json, err := PrettyJSON(obj)
		if err != nil {
			return err
		}
		obj = *json
	}

	fmt.Fprintln(cmd.OutOrStdout(), obj)

	return nil
}

// Header is a customized block header for cli.
type Header struct {
	ParentHash       common.Hash        `json:"parentHash"`
	UncleHash        common.Hash        `json:"sha3Uncles"`
	Coinbase         common.Address     `json:"miner"`
	Hash             common.Hash        `json:"hash"`
	Root             common.Hash        `json:"stateRoot"`
	TxHash           common.Hash        `json:"transactionsRoot"`
	ReceiptHash      common.Hash        `json:"receiptsRoot"`
	Bloom            types.Bloom        `json:"logsBloom"`
	Difficulty       *big.Int           `json:"difficulty"`
	Number           *big.Int           `json:"number"`
	GasLimit         uint64             `json:"gasLimit"`
	GasUsed          uint64             `json:"gasUsed"`
	Time             uint64             `json:"timestamp"`
	Extra            []byte             `json:"extraData"`
	MixDigest        common.Hash        `json:"mixHash"`
	Nonce            types.BlockNonce   `json:"nonce"`
	BaseFee          *big.Int           `json:"baseFeePerGas"`
	WithdrawalsHash  *common.Hash       `json:"withdrawalsRoot"`
	Size             common.StorageSize `json:"size"`
	// TODO: totalDifficulty to be implemented
	// TotalDifficulty  *big.Int           `json:"totalDifficulty"`
	TransactionsHash []common.Hash      `json:"transactions"`
}

// NewHeader returns the custom-built Header object.
func NewHeader(b *types.Block) *Header {
	return &Header{
		ParentHash:       b.Header().ParentHash,
		UncleHash:        b.Header().UncleHash,
		Coinbase:         b.Header().Coinbase,
		Hash:             b.Hash(),
		Root:             b.Header().Root,
		TxHash:           b.Header().TxHash,
		ReceiptHash:      b.ReceiptHash(),
		Bloom:            b.Bloom(),
		Difficulty:       b.Header().Difficulty,
		Number:           b.Header().Number,
		GasLimit:         b.Header().GasLimit,
		GasUsed:          b.Header().GasUsed,
		Time:             b.Header().Time,
		Extra:            b.Header().Extra,
		MixDigest:        b.Header().MixDigest,
		Nonce:            b.Header().Nonce,
		BaseFee:          b.Header().BaseFee,
		WithdrawalsHash:  b.Header().WithdrawalsHash,
		Size:             b.Size(),
		// TotalDifficulty:  b.Difficulty(),
		TransactionsHash: TransactionsHash(*b),
	}
}

// String overrides the standard behavior for Header "to-string".
func (h *Header) String() string {
	var p Printable
	if err := p.FromStruct(h); err != nil {
		panic(err)
	}
	s := p.Columnize(*NewPrintableFormat(20, 0, 0, byte(' ')))

	return s
}

// TransactionsHash returns a list of transaction hash starting from a list of transactions contained in a block.
func TransactionsHash(block types.Block) []common.Hash {
	txsh := make([]common.Hash, len(block.Transactions()))

	for i, tx := range block.Transactions() {
		txsh[i] = tx.Hash()
	}

	return txsh
}

// Block is a customized block for cli.
type Block struct {
	ParentHash      common.Hash        `json:"parentHash"`
	UncleHash       common.Hash        `json:"sha3Uncles"`
	Coinbase        common.Address     `json:"miner"`
	Hash            common.Hash        `json:"hash"`
	Root            common.Hash        `json:"stateRoot"`
	TxHash          common.Hash        `json:"transactionsRoot"`
	ReceiptHash     common.Hash        `json:"receiptsRoot"`
	Bloom           types.Bloom        `json:"logsBloom"`
	Difficulty      *big.Int           `json:"difficulty"`
	Number          *big.Int           `json:"number"`
	GasLimit        uint64             `json:"gasLimit"`
	GasUsed         uint64             `json:"gasUsed"`
	Time            uint64             `json:"timestamp"`
	Extra           []byte             `json:"extraData"`
	MixDigest       common.Hash        `json:"mixHash"`
	Nonce           types.BlockNonce   `json:"nonce"`
	BaseFee         *big.Int           `json:"baseFeePerGas"`
	WithdrawalsHash *common.Hash       `json:"withdrawalsRoot"`
	Size            common.StorageSize `json:"size"`
	// TODO: totalDifficulty to be implemented
	// TotalDifficulty *big.Int           `json:"totalDifficulty"`
	Uncles          []*types.Header    `json:"uncles"`
	Transactions    types.Transactions `json:"transactions"`
	Withdrawals     types.Withdrawals  `json:"withdrawals"`
}

// NewBlock returns the custom-built Block object.
func NewBlock(b *types.Block) *Block {
	return &Block{
		ParentHash:      b.Header().ParentHash,
		UncleHash:       b.Header().UncleHash,
		Coinbase:        b.Header().Coinbase,
		Hash:            b.Hash(),
		Root:            b.Header().Root,
		TxHash:          b.Header().TxHash,
		ReceiptHash:     b.ReceiptHash(),
		Bloom:           b.Bloom(),
		Difficulty:      b.Header().Difficulty,
		Number:          b.Header().Number,
		GasLimit:        b.Header().GasLimit,
		GasUsed:         b.Header().GasUsed,
		Time:            b.Header().Time,
		Extra:           b.Header().Extra,
		MixDigest:       b.Header().MixDigest,
		Nonce:           b.Header().Nonce,
		BaseFee:         b.Header().BaseFee,
		WithdrawalsHash: b.Header().WithdrawalsHash,
		Size:            b.Size(),
		// TotalDifficulty: b.Difficulty(),
		Uncles:          b.Uncles(),
		Transactions:    b.Transactions(),
		// TODO: Withdrawals is empty. To be fixed.
		Withdrawals:     b.Withdrawals(),
	}
}

// String overrides the standard behavior for Block "to-string".
func (b *Block) String() string {
	var p Printable
	if err := p.FromStruct(b); err != nil {
		panic(err)
	}
	s := p.Columnize(*NewPrintableFormat(20, 0, 0, byte(' ')))

	return s
}
