package ethrpc2

import (
	"context"
	"math/big"
	"net/http"

	"github.com/goware/breaker"
	"github.com/goware/cachestore"
	"github.com/goware/logger"
)

type Provider struct {
	log        logger.Logger
	nodeURL    string
	httpClient *http.Client
	br         breaker.Breaker

	chainID *big.Int
	cache   cachestore.Store[[]byte]
}

func NewProvider(nodeURL string, optHTTPClient ...*http.Client) (*Provider, error) {
	return nil, nil
}

func (s *Provider) ChainID(ctx context.Context) (*big.Int, error) {
	return nil, nil
}

func (s *Provider) GetBalance(ctx context.Context) error {
	return nil
}

// TODO: .. each method..
// eth_XXXX

// TODO: lets add batch request support.. may as well
// .Do(.. etc..)

// TODO: add middleware thing.. like provider.With(etc..).BlockByNumber() ..

// JSON-RPC methods..
/*

web3_clientVersion
web3_sha3
net_version
net_listening
net_peerCount
eth_protocolVersion
eth_syncing
eth_coinbase
eth_mining
eth_hashrate
eth_gasPrice
eth_accounts
eth_blockNumber
eth_getBalance
eth_getStorageAt
eth_getTransactionCount
eth_getBlockTransactionCountByHash
eth_getBlockTransactionCountByNumber
eth_getUncleCountByBlockHash
eth_getUncleCountByBlockNumber
eth_getCode
eth_sign
eth_signTransaction
eth_sendTransaction
eth_sendRawTransaction
eth_call
eth_estimateGas
eth_getBlockByHash
eth_getBlockByNumber
eth_getTransactionByHash
eth_getTransactionByBlockHashAndIndex
eth_getTransactionByBlockNumberAndIndex
eth_getTransactionReceipt
eth_getUncleByBlockHashAndIndex
eth_getUncleByBlockNumberAndIndex
eth_getCompilers
eth_compileSolidity
eth_compileLLL
eth_compileSerpent
eth_newFilter
eth_newBlockFilter
eth_newPendingTransactionFilter
eth_uninstallFilter
eth_getFilterChanges
eth_getFilterLogs
eth_getLogs
eth_getWork
eth_submitWork
eth_submitHashrate

*/
