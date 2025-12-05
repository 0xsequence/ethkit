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
	"github.com/0xsequence/ethkit/ethutil"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

const (
	flagBlockField             = "field"
	flagBlockFull              = "full"
	flagBlockRpcUrl            = "rpc-url"
	flagBlockJson              = "json"
	flagBlockCheckLogs         = "check-logs-bloom"
	flagBlockIgnoreZeroGasLogs = "ignore-zero-gas-logs"
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
	cmd.Flags().Bool(flagBlockCheckLogs, false, "Check logs bloom against the block header reported value")
	cmd.Flags().Bool(flagBlockIgnoreZeroGasLogs, false, "Ignore logs from transactions with gas price 0 when checking bloom (HyperEVM system txs)")

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
	fBlockCheckLogs, err := cmd.Flags().GetBool(flagBlockCheckLogs)
	if err != nil {
		return err
	}
	fIgnoreZeroGasLogs, err := cmd.Flags().GetBool(flagBlockIgnoreZeroGasLogs)
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
		return fmt.Errorf("error: invalid block height: %w", err)
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

	if fBlockCheckLogs {
		CheckLogs(block, provider, fIgnoreZeroGasLogs)
	}

	fmt.Fprintln(cmd.OutOrStdout(), obj)

	return nil
}

// Header is a customized block header for cli.
type Header struct {
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
	// TotalDifficulty  *big.Int           `json:"totalDifficulty"`
	TransactionsHash []common.Hash `json:"transactions"`
}

// NewHeader returns the custom-built Header object.
func NewHeader(b *types.Block) *Header {
	return &Header{
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
		Size:            b.Header().Size(),
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
	Uncles       []*types.Header    `json:"uncles"`
	Transactions types.Transactions `json:"transactions"`
	Withdrawals  types.Withdrawals  `json:"withdrawals"`
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
		Size:            b.Header().Size(),
		// TotalDifficulty: b.Difficulty(),
		Uncles:       b.Uncles(),
		Transactions: b.Transactions(),
		// TODO: Withdrawals is empty. To be fixed.
		Withdrawals: b.Withdrawals(),
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

// CheckLogs verifies that the logs bloom and logs hash in the block header match the actual logs
func CheckLogs(block *types.Block, provider *ethrpc.Provider, ignoreZeroGasLogs bool) {
	h, err := provider.HeaderByNumber(context.Background(), block.Number())

	if err != nil {
		fmt.Println("Error getting header:", err)
	}

	logs, err := provider.FilterLogs(context.Background(), ethereum.FilterQuery{
		FromBlock: block.Number(),
		ToBlock:   block.Number(),
	})

	if err != nil {
		fmt.Println("Error getting logs:", err)
	}

	filteredLogs := logs
	var optCheck []ethutil.LogsBloomCheckFunc

	if ignoreZeroGasLogs {
		// Build a quick lookup of tx hash -> gas price so we can drop system (zero price) tx logs.
		gasPriceByTx := make(map[common.Hash]*big.Int, len(block.Transactions()))
		for _, tx := range block.Transactions() {
			gasPriceByTx[tx.Hash()] = tx.GasPrice()
		}

		filter := func(ls []types.Log) []types.Log {
			out := make([]types.Log, 0, len(ls))
			for _, l := range ls {
				if gp, ok := gasPriceByTx[l.TxHash]; ok && gp.Sign() == 0 {
					// HyperEVM system tx (gas price = 0) — ignore for bloom validation.
					continue
				}
				out = append(out, l)
			}
			return out
		}

		filteredLogs = filter(logs)

		optCheck = append(optCheck, func(ls []types.Log, header *types.Header) bool {
			return ethutil.ValidateLogsWithBlockHeader(filter(ls), header)
		})
	}

	fmt.Printf("Block: %d\n", h.Number.Uint64())
	fmt.Printf("Logs Count: %d\n", len(filteredLogs))
	fmt.Printf("Match: %v\n", ethutil.ValidateLogsWithBlockHeader(filteredLogs, h, optCheck...))
	fmt.Println()
	fmt.Printf("Calculated Log Bloom: 0x%x\n", ethutil.ConvertLogsToBloom(filteredLogs).Bytes())
	fmt.Println()
	fmt.Printf("Header Log Bloom: 0x%x\n", h.Bloom.Bytes())
	fmt.Println()
}
