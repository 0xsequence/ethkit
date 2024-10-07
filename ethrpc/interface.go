package ethrpc

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// Standard Ethereum JSON-RPC methods:
// https://ethereum.org/en/developers/docs/apis/json-rpc/
//
// web3_clientVersion
// web3_sha3
// net_version
// net_listening
// net_peerCount
// eth_protocolVersion
// eth_syncing
// eth_coinbase
// eth_mining
// eth_hashrate
// eth_gasPrice
// eth_accounts
// eth_blockNumber
// eth_getBalance
// eth_getStorageAt
// eth_getTransactionCount
// eth_getBlockTransactionCountByHash
// eth_getBlockTransactionCountByNumber
// eth_getUncleCountByBlockHash
// eth_getUncleCountByBlockNumber
// eth_getCode
// eth_sign
// eth_signTransaction
// eth_sendTransaction
// eth_sendRawTransaction
// eth_call
// eth_estimateGas
// eth_getBlockByHash
// eth_getBlockByNumber
// eth_getTransactionByHash
// eth_getTransactionByBlockHashAndIndex
// eth_getTransactionByBlockNumberAndIndex
// eth_getTransactionReceipt
// eth_getUncleByBlockHashAndIndex
// eth_getUncleByBlockNumberAndIndex
// eth_getCompilers
// eth_compileSolidity
// eth_compileLLL
// eth_compileSerpent
// eth_newFilter
// eth_newBlockFilter
// eth_newPendingTransactionFilter
// eth_uninstallFilter
// eth_getFilterChanges
// eth_getFilterLogs
// eth_getLogs
// eth_getWork
// eth_submitWork
// eth_submitHashrate

// TODO: rename to either Provider, and rename the current Provider to Client
type Interface interface {
	// ..
	Do(ctx context.Context, calls ...Call) ([]byte, error)

	// ChainID = eth_chainId
	ChainID(ctx context.Context) (*big.Int, error)

	// BlockByHash = eth_getBlockByHash (true)
	BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error)

	// BlockByNumber = eth_getBlockByNumber (true)
	BlockByNumber(ctx context.Context, blockNum *big.Int) (*types.Block, error)

	// BlockNumber = eth_blockNumber
	BlockNumber(ctx context.Context) (uint64, error)

	// PeerCount = net_peerCount
	PeerCount(ctx context.Context) (uint64, error)

	// HeaderByHash = eth_getBlockByHash (false)
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)

	// HeaderByNumber = eth_getBlockByHash (true)
	HeaderByNumber(ctx context.Context, blockNum *big.Int) (*types.Header, error)

	// HeadersByNumbers = batch of eth_getHeaderByNumber
	HeadersByNumbers(ctx context.Context, blockNumbers []*big.Int) ([]*types.Header, error)

	// HeadersByNumberRange = batch of eth_getHeaderByNumber
	HeadersByNumberRange(ctx context.Context, fromBlockNumber, toBlockNumber *big.Int) ([]*types.Header, error)

	// TransactionByHash = eth_getTransactionByHash
	TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, pending bool, err error)

	// TransactionSender is a wrapper for eth_getTransactionByBlockHashAndIndex
	TransactionSender(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error)

	// TransactionCount = eth_getBlockTransactionCountByHash
	TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error)

	// TransactionInBlock = eth_getTransactionByBlockHashAndIndex
	TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error)

	// TransactionReceipt = eth_getTransactionReceipt
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)

	// SyncProgress = eth_syncing
	SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error)

	// NetworkID = net_version
	NetworkID(ctx context.Context) (*big.Int, error)

	// BalanceAt = eth_getBalance
	BalanceAt(ctx context.Context, account common.Address, blockNum *big.Int) (*big.Int, error)

	// StorageAt = eth_getStorageAt
	StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNum *big.Int) ([]byte, error)

	// CodeAt = eth_getCode
	CodeAt(ctx context.Context, account common.Address, blockNum *big.Int) ([]byte, error)

	// NonceAt = eth_getTransactionCount
	NonceAt(ctx context.Context, account common.Address, blockNum *big.Int) (uint64, error)

	// FilterLogs = eth_getLogs
	FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)

	// PendingBalanceAt = eth_getBalance ("pending")
	PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error)

	// PendingStorageAt = eth_getStorageAt ("pending")
	PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error)

	// PendingCodeAt = eth_getCode ("pending")
	PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error)

	// PendingNonceAt = eth_getTransactionCount ("pending")
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)

	// PendingTransactionCount = eth_getBlockTransactionCountByNumber ("pending")
	PendingTransactionCount(ctx context.Context) (uint, error)

	// CallContract = eth_call (blockNumber)
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNum *big.Int) ([]byte, error)

	// CallContractAtHash = eth_call (blockHash)
	CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error)

	// PendingCallContract = eth_call ("pending")
	PendingCallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error)

	// SuggestGasPrice = eth_gasPrice
	SuggestGasPrice(ctx context.Context) (*big.Int, error)

	// SuggestGasTipCap = eth_maxPriorityFeePerGas
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)

	// FeeHistory = eth_feeHistory
	FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error)

	// EstimateGas = eth_estimateGas
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)

	// SendTransaction = eth_sendRawTransaction
	SendTransaction(ctx context.Context, tx *types.Transaction) error

	// SendRawTransaction = eth_sendRawTransaction
	SendRawTransaction(ctx context.Context, signedTxHex string) (common.Hash, error)

	// ..
	IsStreamingEnabled() bool

	// ..
	SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)

	// ..
	SubscribeNewHeads(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)

	// ..
	CloseStreamConns()
}

// RawInterface also returns the bytes of the response body payload
type RawInterface interface {
	Interface
	RawBlockByHash(ctx context.Context, hash common.Hash) (json.RawMessage, error)
	RawBlockByNumber(ctx context.Context, blockNum *big.Int) (json.RawMessage, error)
	RawFilterLogs(ctx context.Context, q ethereum.FilterQuery) (json.RawMessage, error)
}
