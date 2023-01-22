package ethrpc2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync/atomic"

	"github.com/goware/breaker"
	"github.com/goware/cachestore"
	"github.com/goware/logger"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Provider struct {
	log        logger.Logger
	nodeURL    string
	httpClient httpClient
	br         breaker.Breaker

	chainID *big.Int
	cache   cachestore.Store[[]byte]
	lastID  atomic.Uint32
}

func NewProvider(ctx context.Context, nodeURL string, options ...Option) (*Provider, error) {
	p := &Provider{
		nodeURL:    nodeURL,
		httpClient: http.DefaultClient,
	}

	for _, opt := range options {
		opt(p)
	}

	var err error
	p.chainID, err = p.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) Do(ctx context.Context, calls ...Call) error {
	if len(calls) == 0 {
		return nil
	}

	batchReq := make([]*jsonrpcMessage, 0, len(calls))
	for i, call := range calls {
		if call.err != nil {
			// TODO: store and return the error but execute the rest of the batch?
			return fmt.Errorf("call %d has an error: %w", i, call.err)
		}

		jrpcReq := call.request
		jrpcReq.ID = p.lastID.Add(1)
		batchReq = append(batchReq, jrpcReq)
	}

	var reqBody any = batchReq
	if len(batchReq) == 1 {
		reqBody = batchReq[0]
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal JSONRPC request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, p.nodeURL, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("failed to initialize http.Request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	res, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer res.Body.Close()

	var (
		results []jsonrpcMessage
		target  any = &results
	)
	if len(batchReq) == 1 {
		results = make([]jsonrpcMessage, 1)
		target = &results[0]
	}

	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	for i, result := range results {
		// TODO: handle batch errors
		if e := result.Error; e != nil {
			return fmt.Errorf("JSONRPC error %d: %s", e.Code, e.Message)
		}
		if err := calls[i].resultFn(result.Result); err != nil {
			return fmt.Errorf("failed to store result value: %w", err)
		}
	}

	return nil
}

var _ Interface = (*Provider)(nil)

func (p *Provider) ChainID(ctx context.Context) (*big.Int, error) {
	var ret *big.Int
	err := p.Do(ctx, ChainID().Into(&ret))
	return ret, err
}

func (p *Provider) BlockNumber(ctx context.Context) (uint64, error) {
	var ret uint64
	err := p.Do(ctx, BlockNumber().Into(&ret))
	return ret, err
}

func (p *Provider) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	var ret *big.Int
	err := p.Do(ctx, BalanceAt(account, blockNumber).Into(&ret))
	return ret, err
}

func (p *Provider) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return p.Do(ctx, SendTransaction(tx))
}

func (p *Provider) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	var ret *types.Block
	err := p.Do(ctx, BlockByHash(hash).Into(&ret))
	return ret, err
}

func (p *Provider) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	var ret *types.Block
	err := p.Do(ctx, BlockByNumber(number).Into(&ret))
	return ret, err
}

func (p *Provider) PeerCount(ctx context.Context) (uint64, error) {
	var ret uint64
	err := p.Do(ctx, PeerCount().Into(&ret))
	return ret, err
}

func (p *Provider) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	var head *types.Header
	err := p.Do(ctx, HeaderByHash(hash).Into(&head))
	return head, err
}

func (p *Provider) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	var head *types.Header
	err := p.Do(ctx, HeaderByNumber(number).Into(&head))
	return head, err
}

func (p *Provider) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, pending bool, err error) {
	err = p.Do(ctx, TransactionByHash(hash).Into(&tx, &pending))
	return tx, pending, err
}

func (p *Provider) TransactionSender(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error) {
	sender, err := types.Sender(&senderFromServer{blockhash: block}, tx)
	if err != nil {
		return sender, nil
	}
	err = p.Do(ctx, TransactionSender(tx, block, index).Into(&sender))
	return sender, err
}

func (p *Provider) TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error) {
	var ret uint
	err := p.Do(ctx, TransactionCount(blockHash).Into(&ret))
	return ret, err
}

func (p *Provider) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
	var tx *types.Transaction
	err := p.Do(ctx, TransactionInBlock(blockHash, index).Into(&tx))
	return tx, err
}

func (p *Provider) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := p.Do(ctx, TransactionReceipt(txHash).Into(&receipt))
	return receipt, err
}

func (p *Provider) SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error) {
	var progress *ethereum.SyncProgress
	err := p.Do(ctx, SyncProgress().Into(&progress))
	return progress, err
}

func (p *Provider) NetworkID(ctx context.Context) (*big.Int, error) {
	var version *big.Int
	err := p.Do(ctx, NetworkID().Into(&version))
	return version, err
}

func (p *Provider) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	var result []byte
	err := p.Do(ctx, StorageAt(account, key, blockNumber).Into(&result))
	return result, err
}

func (p *Provider) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	var result []byte
	err := p.Do(ctx, CodeAt(account, blockNumber).Into(&result))
	return result, err
}

func (p *Provider) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	var result uint64
	err := p.Do(ctx, NonceAt(account, blockNumber).Into(&result))
	return result, err
}

func (p *Provider) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	var logs []types.Log
	err := p.Do(ctx, FilterLogs(q).Into(&logs))
	return logs, err
}

func (p *Provider) PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	var ret *big.Int
	err := p.Do(ctx, PendingBalanceAt(account).Into(&ret))
	return ret, err
}

func (p *Provider) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
	var result []byte
	err := p.Do(ctx, PendingStorageAt(account, key).Into(&result))
	return result, err
}

func (p *Provider) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	var result []byte
	err := p.Do(ctx, PendingCodeAt(account).Into(&result))
	return result, err
}

func (p *Provider) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	var result uint64
	err := p.Do(ctx, PendingNonceAt(account).Into(&result))
	return result, err
}

func (p *Provider) PendingTransactionCount(ctx context.Context) (uint, error) {
	var ret uint
	err := p.Do(ctx, PendingTransactionCount().Into(&ret))
	return ret, err
}

func (p *Provider) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	var result []byte
	err := p.Do(ctx, CallContract(msg, blockNumber).Into(&result))
	return result, err
}

func (p *Provider) CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error) {
	var result []byte
	err := p.Do(ctx, CallContractAtHash(msg, blockHash).Into(&result))
	return result, err
}

func (p *Provider) PendingCallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	var result []byte
	err := p.Do(ctx, PendingCallContract(msg).Into(&result))
	return result, err
}

func (p *Provider) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	var ret *big.Int
	err := p.Do(ctx, SuggestGasPrice().Into(&ret))
	return ret, err
}

func (p *Provider) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	var ret *big.Int
	err := p.Do(ctx, SuggestGasTipCap().Into(&ret))
	return ret, err
}

func (p *Provider) FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
	var fh *ethereum.FeeHistory
	err := p.Do(ctx, FeeHistory(blockCount, lastBlock, rewardPercentiles).Into(&fh))
	return fh, err
}

func (p *Provider) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	var result uint64
	err := p.Do(ctx, EstimateGas(msg).Into(&result))
	return result, err
}
