package ethrpc

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/rpc"
)

func ChainID() CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "eth_chainId",
		intoFn: hexIntoBigUint64,
	}
}

func BlockNumber() CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "eth_blockNumber",
		intoFn: hexIntoUint64,
	}
}

func BalanceAt(account common.Address, blockNum *big.Int) CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "eth_getBalance",
		params: []any{account, toBlockNumArg(blockNum)},
		intoFn: hexIntoBigInt,
	}
}

func SendTransaction(tx *types.Transaction) Call {
	data, err := tx.MarshalBinary()
	if err != nil {
		return Call{err: err}
	}
	return Call{
		request: jsonrpc.NewRequest(0, "eth_sendRawTransaction", []any{hexutil.Encode(data)}),
		// NOTE: we don't care about the result..?
		// TODO: this method will return the txnHash, so this feels wrong. I believe this is what
		// geth does, but we can use SendRawTransaction instead
		resultFn: nil,
	}
}

func SendRawTransaction(signedTxHex string) CallBuilder[common.Hash] {
	return CallBuilder[common.Hash]{
		method: "eth_sendRawTransaction",
		params: []any{signedTxHex},
		intoFn: hexIntoHash,
	}
}

func BlockByHash(hash common.Hash) CallBuilder[*types.Block] {
	return CallBuilder[*types.Block]{
		method: "eth_getBlockByHash",
		params: []any{hash, true},
		intoFn: IntoBlock,
	}
}

func BlockByNumber(blockNum *big.Int) CallBuilder[*types.Block] {
	return CallBuilder[*types.Block]{
		method: "eth_getBlockByNumber",
		params: []any{toBlockNumArg(blockNum), true},
		intoFn: IntoBlock,
	}
}

func RawBlockByHash(hash common.Hash) CallBuilder[json.RawMessage] {
	return CallBuilder[json.RawMessage]{
		method: "eth_getBlockByHash",
		params: []any{hash, true},
		intoFn: IntoJSONRawMessage,
	}
}

func RawBlockByNumber(blockNum *big.Int) CallBuilder[json.RawMessage] {
	return CallBuilder[json.RawMessage]{
		method: "eth_getBlockByNumber",
		params: []any{toBlockNumArg(blockNum), true},
		intoFn: IntoJSONRawMessage,
	}
}

func PeerCount() CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "net_peerCount",
		intoFn: hexIntoUint64,
	}
}

func HeaderByHash(hash common.Hash) CallBuilder[*types.Header] {
	return CallBuilder[*types.Header]{
		method: "eth_getBlockByHash",
		params: []any{hash, false},
		intoFn: IntoHeader,
	}
}

func HeaderByNumber(blockNum *big.Int) CallBuilder[*types.Header] {
	return CallBuilder[*types.Header]{
		method: "eth_getBlockByNumber",
		params: []any{toBlockNumArg(blockNum), false},
		intoFn: IntoHeader,
	}
}

func TransactionByHash(hash common.Hash) CallBuilder2[*types.Transaction, bool] {
	return CallBuilder2[*types.Transaction, bool]{
		method: "eth_getTransactionByHash",
		params: []any{hash},
		intoFn: IntoTransactionWithPending,
	}
}

func TransactionSender(tx *types.Transaction, block common.Hash, index uint) CallBuilder[common.Address] {
	return CallBuilder[common.Address]{
		method: "eth_getTransactionByBlockHashAndIndex",
		params: []any{block, hexutil.Uint64(index)},
		intoFn: func(raw json.RawMessage, ret *common.Address, strictness StrictnessLevel) error {
			var meta struct {
				Hash common.Hash
				From common.Address
			}
			if err := json.Unmarshal(raw, &meta); err != nil {
				return err
			}
			if meta.Hash == (common.Hash{}) || meta.Hash != tx.Hash() {
				return fmt.Errorf("wrong inclusion block/index")
			}
			*ret = meta.From
			return nil
		},
	}
}

func TransactionCount(blockHash common.Hash) CallBuilder[uint] {
	return CallBuilder[uint]{
		method: "eth_getBlockTransactionCountByHash",
		params: []any{blockHash},
		intoFn: hexIntoUint,
	}
}

func TransactionInBlock(blockHash common.Hash, index uint) CallBuilder[*types.Transaction] {
	return CallBuilder[*types.Transaction]{
		method: "eth_getTransactionByBlockHashAndIndex",
		params: []any{blockHash, hexutil.Uint64(index)},
		intoFn: IntoTransaction,
	}
}

func TransactionReceipt(txHash common.Hash) CallBuilder[*types.Receipt] {
	return CallBuilder[*types.Receipt]{
		method: "eth_getTransactionReceipt",
		params: []any{txHash},
		intoFn: func(raw json.RawMessage, receipt **types.Receipt, strictness StrictnessLevel) error {
			err := json.Unmarshal(raw, receipt)
			if err == nil && receipt == nil {
				return ethereum.NotFound
			}
			return err
		},
	}
}

func SyncProgress() CallBuilder[*ethereum.SyncProgress] {
	return CallBuilder[*ethereum.SyncProgress]{
		method: "eth_syncing",
		intoFn: intoSyncingProgress,
	}
}

func NetworkID() CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "net_version",
		intoFn: func(raw json.RawMessage, ret **big.Int, strictness StrictnessLevel) error {
			var (
				verString string
				version   = &big.Int{}
			)
			if err := json.Unmarshal(raw, &verString); err != nil {
				return err
			}
			if _, ok := version.SetString(verString, 10); !ok {
				return fmt.Errorf("invalid net_version result: %q", verString)
			}
			*ret = version
			return nil
		},
	}
}

func StorageAt(account common.Address, key common.Hash, blockNum *big.Int) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_getStorageAt",
		params: []any{account, key, toBlockNumArg(blockNum)},
		intoFn: hexIntoBytes,
	}
}

func CodeAt(account common.Address, blockNum *big.Int) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_getCode",
		params: []any{account, toBlockNumArg(blockNum)},
		intoFn: hexIntoBytes,
	}
}

func NonceAt(account common.Address, blockNum *big.Int) CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "eth_getTransactionCount",
		params: []any{account, toBlockNumArg(blockNum)},
		intoFn: hexIntoUint64,
	}
}

func FilterLogs(q ethereum.FilterQuery) CallBuilder[[]types.Log] {
	arg, err := toFilterArg(q)
	if err != nil {
		return CallBuilder[[]types.Log]{err: err}
	}
	return CallBuilder[[]types.Log]{
		method: "eth_getLogs",
		params: []any{arg},
	}
}

func RawFilterLogs(q ethereum.FilterQuery) CallBuilder[json.RawMessage] {
	arg, err := toFilterArg(q)
	if err != nil {
		return CallBuilder[json.RawMessage]{err: err}
	}
	return CallBuilder[json.RawMessage]{
		method: "eth_getLogs",
		params: []any{arg},
	}
}

func PendingBalanceAt(account common.Address) CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "eth_getBalance",
		params: []any{account, "pending"},
		intoFn: hexIntoBigInt,
	}
}

func PendingStorageAt(account common.Address, key common.Hash) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_getStorageAt",
		params: []any{account, key, "pending"},
		intoFn: hexIntoBytes,
	}
}

func PendingCodeAt(account common.Address) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_getCode",
		params: []any{account, "pending"},
		intoFn: hexIntoBytes,
	}
}

func PendingNonceAt(account common.Address) CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "eth_getTransactionCount",
		params: []any{account, "pending"},
		intoFn: hexIntoUint64,
	}
}

func PendingTransactionCount() CallBuilder[uint] {
	return CallBuilder[uint]{
		method: "eth_getBlockTransactionCountByNumber",
		params: []any{"pending"},
		intoFn: hexIntoUint,
	}
}

func ContractQuery(contractAddress common.Address, inputAbiExpr, outputAbiExpr string, args interface{}) (CallBuilder[[]string], error) {
	var (
		calldata []byte
		err      error
	)

	switch args := args.(type) {
	case []string:
		calldata, err = ethcoder.ABIEncodeMethodCalldataFromStringValues(inputAbiExpr, args)
		if err != nil {
			return CallBuilder[[]string]{}, fmt.Errorf("abi encode failed: %w", err)
		}

	case []interface{}:
		calldata, err = ethcoder.ABIEncodeMethodCalldata(inputAbiExpr, args)
		if err != nil {
			return CallBuilder[[]string]{}, fmt.Errorf("abi encode failed: %w", err)
		}
	case nil:
		calldata, err = ethcoder.ABIEncodeMethodCalldata(inputAbiExpr, nil)
		if err != nil {
			return CallBuilder[[]string]{}, fmt.Errorf("abi encode failed: %w", err)
		}
	}

	msg := ethereum.CallMsg{
		To:   &contractAddress,
		Data: calldata,
	}

	return CallBuilder[[]string]{
		method: "eth_call",
		params: []any{toCallArg(msg), toBlockNumArg(nil)},
		intoFn: func(message json.RawMessage, ret *[]string, strictness StrictnessLevel) error {
			var result hexutil.Bytes
			if err := json.Unmarshal(message, &result); err != nil {
				return err
			}

			resp, err := ethcoder.ABIUnpackAndStringify(outputAbiExpr, result)
			if err != nil {
				return fmt.Errorf("abi decode of response failed: %w", err)
			}
			*ret = resp
			return nil
		},
	}, nil
}

func CallContract(msg ethereum.CallMsg, blockNum *big.Int) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_call",
		params: []any{toCallArg(msg), toBlockNumArg(blockNum)},
		intoFn: hexIntoBytes,
	}
}

func CallContractAtHash(msg ethereum.CallMsg, blockHash common.Hash) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_call",
		params: []any{toCallArg(msg), rpc.BlockNumberOrHashWithHash(blockHash, false)},
		intoFn: hexIntoBytes,
	}
}

func PendingCallContract(msg ethereum.CallMsg) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_call",
		params: []any{toCallArg(msg), "pending"},
		intoFn: hexIntoBytes,
	}
}

func SuggestGasPrice() CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "eth_gasPrice",
		intoFn: hexIntoBigInt,
	}
}

func SuggestGasTipCap() CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "eth_maxPriorityFeePerGas",
		intoFn: hexIntoBigInt,
	}
}

type feeHistoryResult struct {
	OldestBlock  *hexutil.Big     `json:"oldestBlock"`
	Reward       [][]*hexutil.Big `json:"reward,omitempty"`
	BaseFee      []*hexutil.Big   `json:"baseFeePerGas,omitempty"`
	GasUsedRatio []float64        `json:"gasUsedRatio"`
}

func FeeHistory(blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) CallBuilder[*ethereum.FeeHistory] {
	return CallBuilder[*ethereum.FeeHistory]{
		method: "eth_feeHistory",
		params: []any{hexutil.Uint(blockCount), toBlockNumArg(lastBlock), rewardPercentiles},
		intoFn: func(raw json.RawMessage, ret **ethereum.FeeHistory, strictness StrictnessLevel) error {
			var res feeHistoryResult
			if err := json.Unmarshal(raw, &res); err != nil {
				return err
			}

			reward := make([][]*big.Int, len(res.Reward))
			for i, r := range res.Reward {
				reward[i] = make([]*big.Int, len(r))
				for j, r := range r {
					reward[i][j] = (*big.Int)(r)
				}
			}
			baseFee := make([]*big.Int, len(res.BaseFee))
			for i, b := range res.BaseFee {
				baseFee[i] = (*big.Int)(b)
			}
			*ret = &ethereum.FeeHistory{
				OldestBlock:  (*big.Int)(res.OldestBlock),
				Reward:       reward,
				BaseFee:      baseFee,
				GasUsedRatio: res.GasUsedRatio,
			}
			return nil
		},
	}
}

func EstimateGas(msg ethereum.CallMsg) CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "eth_estimateGas",
		params: []any{toCallArg(msg)},
		intoFn: hexIntoUint64,
	}
}

type EthSimulatePayload struct {
	BlockStateCalls        []BlockStateCall `json:"blockStateCalls"`
	TraceTransfers         bool             `json:"traceTransfers,omitempty"`
	Validation             bool             `json:"validation,omitempty"`
	ReturnFullTransactions bool             `json:"returnFullTransactions,omitempty"`
}

type BlockStateCall struct {
	BlockOverrides interface{}              `json:"blockOverrides,omitempty"`
	StateOverrides map[string]StateOverride `json:"stateOverrides,omitempty"`
	Calls          []GenericCallTransaction `json:"calls"`
}

type StateOverride struct {
	Balance *hexutil.Big                `json:"balance,omitempty"`
	Nonce   *hexutil.Uint64             `json:"nonce,omitempty"`
	Code    *hexutil.Bytes              `json:"code,omitempty"`
	Storage map[common.Hash]common.Hash `json:"state,omitempty"`
}

type GenericCallTransaction struct {
	From         common.Address `json:"from"`
	To           common.Address `json:"to"`
	Gas          string         `json:"gas,omitempty"`
	GasPrice     string         `json:"gasPrice,omitempty"`
	MaxFeePerGas string         `json:"maxFeePerGas,omitempty"`
	Value        string         `json:"value,omitempty"`
	Data         hexutil.Bytes  `json:"data,omitempty"`
}

type SimulatedCall struct {
	Status     string          `json:"status"` // "0x1" for success, "0x0" for failure
	ReturnData hexutil.Bytes   `json:"returnData"`
	GasUsed    hexutil.Uint64  `json:"gasUsed"`
	Error      *SimulatedError `json:"error,omitempty"`
	Logs       []CallResultLog `json:"logs,omitempty"`
}

type SimulatedError struct {
	Code    uint64        `json:"code"`
	Message string        `json:"message"`
	Data    hexutil.Bytes `json:"data"`
}

type CallResultLog struct {
	LogIndex         hexutil.Uint64 `json:"logIndex"`
	BlockHash        common.Hash    `json:"blockHash"`
	BlockNumber      hexutil.Uint64 `json:"blockNumber"`
	TransactionHash  common.Hash    `json:"transactionHash"`
	TransactionIndex hexutil.Uint64 `json:"transactionIndex"`
	Address          common.Address `json:"address"`
	Data             hexutil.Bytes  `json:"data"`
	Topics           []common.Hash  `json:"topics"`
	Removed          bool           `json:"removed"`
}

type SimulatedBlock struct {
	BaseFeePerGas   hexutil.Big     `json:"baseFeePerGas"`
	Difficulty      hexutil.Big     `json:"difficulty"`
	ExtraData       hexutil.Bytes   `json:"extraData"`
	GasLimit        hexutil.Uint64  `json:"gasLimit"`
	GasUsed         hexutil.Uint64  `json:"gasUsed"`
	Hash            common.Hash     `json:"hash"`
	LogsBloom       hexutil.Bytes   `json:"logsBloom"`
	Miner           common.Address  `json:"miner"`
	MixHash         common.Hash     `json:"mixHash"`
	Nonce           hexutil.Uint64  `json:"nonce"`
	Number          hexutil.Big     `json:"number"`
	ParentHash      common.Hash     `json:"parentHash"`
	ReceiptsRoot    common.Hash     `json:"receiptsRoot"`
	Sha3Uncles      common.Hash     `json:"sha3Uncles"`
	Size            hexutil.Uint64  `json:"size"`
	StateRoot       common.Hash     `json:"stateRoot"`
	Timestamp       hexutil.Uint64  `json:"timestamp"`
	TotalDifficulty hexutil.Big     `json:"totalDifficulty"`
	Transactions    []string        `json:"transactions"`
	Calls           []SimulatedCall `json:"calls"`
}

func SimulateV1(payload EthSimulatePayload) CallBuilder[[]*SimulatedBlock] {
	return CallBuilder[[]*SimulatedBlock]{
		method: "eth_simulateV1",
		params: []any{payload},
		intoFn: func(raw json.RawMessage, ret *[]*SimulatedBlock, strictness StrictnessLevel) error {
			var blocks []*SimulatedBlock
			if err := json.Unmarshal(raw, &blocks); err != nil {
				return fmt.Errorf("eth_simulateV1 unmarshal failed: %w", err)
			}
			*ret = blocks
			return nil
		},
	}
}

type DebugTracer string

const (
	DebugTracerCallTracer     DebugTracer = "callTracer"
	DebugTracerPreStateTracer DebugTracer = "prestateTracer"
)

type debugTracerOptions struct {
	Name string `json:"tracer"`
}

type CallDebugTrace struct {
	Type         string            `json:"type"`
	From         common.Address    `json:"from"`
	To           common.Address    `json:"to"`
	Value        *hexutil.Big      `json:"value"`
	Gas          *hexutil.Big      `json:"gas"`
	GasUsed      *hexutil.Big      `json:"gasUsed"`
	Input        hexutil.Bytes     `json:"input"`
	Output       hexutil.Bytes     `json:"output"`
	Error        string            `json:"error"`
	RevertReason string            `json:"revertReason"`
	Calls        []*CallDebugTrace `json:"calls"`
}

type TransactionDebugTrace struct {
	TxHash common.Hash    `json:"txHash"`
	Result CallDebugTrace `json:"result"`
}

func DebugTraceBlockByNumber(blockNum *big.Int) CallBuilder[[]*TransactionDebugTrace] {
	return CallBuilder[[]*TransactionDebugTrace]{
		method: "debug_traceBlockByNumber",
		params: []any{toBlockNumArg(blockNum), debugTracerOptions{Name: string(DebugTracerCallTracer)}},
	}
}

func DebugTraceBlockByHash(hash common.Hash) CallBuilder[[]*TransactionDebugTrace] {
	return CallBuilder[[]*TransactionDebugTrace]{
		method: "debug_traceBlockByHash",
		params: []any{hash, debugTracerOptions{Name: string(DebugTracerCallTracer)}},
	}
}

func DebugTraceTransaction(txHash common.Hash) CallBuilder[*CallDebugTrace] {
	return CallBuilder[*CallDebugTrace]{
		method: "debug_traceTransaction",
		params: []any{txHash, debugTracerOptions{Name: string(DebugTracerCallTracer)}},
	}
}
