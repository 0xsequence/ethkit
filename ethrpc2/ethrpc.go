package ethrpc2

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/0xsequence/ethkit/ethrpc2/jsonrpc"
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

func BalanceAt(account common.Address, blockNumber *big.Int) CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "eth_getBalance",
		params: []any{account, toBlockNumArg(blockNumber)},
	}
}

func SendTransaction(tx *types.Transaction) Call {
	data, err := tx.MarshalBinary()
	if err != nil {
		return Call{err: err}
	}
	return Call{
		request:  jsonrpc.NewRequest(nil, "eth_sendRawTransaction", []any{hexutil.Encode(data)}),
		resultFn: nil,
	}
}

func BlockByHash(hash common.Hash) CallBuilder[*types.Block] {
	return CallBuilder[*types.Block]{
		method: "eth_getBlockByHash",
		params: []any{hash, true},
		intoFn: intoBlock,
	}
}

func BlockByNumber(number *big.Int) CallBuilder[*types.Block] {
	return CallBuilder[*types.Block]{
		method: "eth_getBlockByNumber",
		params: []any{toBlockNumArg(number), true},
		intoFn: intoBlock,
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
	}
}

func HeaderByNumber(number *big.Int) CallBuilder[*types.Header] {
	return CallBuilder[*types.Header]{
		method: "eth_getBlockByNumber",
		params: []any{toBlockNumArg(number), false},
	}
}

func TransactionByHash(hash common.Hash) CallBuilder2[*types.Transaction, bool] {
	return CallBuilder2[*types.Transaction, bool]{
		method: "eth_getTransactionByHash",
		params: []any{hash},
		intoFn: intoTransactionWithPending,
	}
}

func TransactionSender(tx *types.Transaction, block common.Hash, index uint) CallBuilder[common.Address] {
	return CallBuilder[common.Address]{
		method: "eth_getTransactionByBlockHashAndIndex",
		params: []any{block, hexutil.Uint64(index)},
		intoFn: func(raw json.RawMessage, ret *common.Address) error {
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
		intoFn: intoTransaction,
	}
}

func TransactionReceipt(txHash common.Hash) CallBuilder[*types.Receipt] {
	return CallBuilder[*types.Receipt]{
		method: "eth_getTransactionReceipt",
		params: []any{txHash},
		intoFn: func(raw json.RawMessage, receipt **types.Receipt) error {
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
		intoFn: func(raw json.RawMessage, ret **big.Int) error {
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

func StorageAt(account common.Address, key common.Hash, blockNumber *big.Int) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_getStorageAt",
		params: []any{account, key, toBlockNumArg(blockNumber)},
		intoFn: hexIntoBytes,
	}
}

func CodeAt(account common.Address, blockNumber *big.Int) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_getCode",
		params: []any{account, toBlockNumArg(blockNumber)},
		intoFn: hexIntoBytes,
	}
}

func NonceAt(account common.Address, blockNumber *big.Int) CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "eth_getTransactionCount",
		params: []any{account, toBlockNumArg(blockNumber)},
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
		params: []any{account},
		intoFn: hexIntoBytes,
	}
}

func PendingNonceAt(account common.Address) CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "eth_getTransactionCount",
		params: []any{account},
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

func CallContract(msg ethereum.CallMsg, blockNumber *big.Int) CallBuilder[[]byte] {
	return CallBuilder[[]byte]{
		method: "eth_call",
		params: []any{toCallArg(msg), toBlockNumArg(blockNumber)},
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
		intoFn: func(raw json.RawMessage, ret **ethereum.FeeHistory) error {
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
