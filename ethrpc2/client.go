package ethrpc2

import (
	"context"
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func ChainID() CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "eth_chainId",
		intoFn: hexIntoBigUint64,
	}
}

func (p *Provider) ChainID(ctx context.Context) (*big.Int, error) {
	var ret *big.Int
	err := p.Do(ctx, ChainID().Into(&ret))
	return ret, err
}

func BlockNumber() CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "eth_blockNumber",
		intoFn: hexIntoUint64,
	}
}

func (p *Provider) BlockNumber(ctx context.Context) (uint64, error) {
	var ret uint64
	err := p.Do(ctx, BlockNumber().Into(&ret))
	return ret, err
}

func BalanceAt(account common.Address, blockNumber *big.Int) CallBuilder[*big.Int] {
	return CallBuilder[*big.Int]{
		method: "eth_getBalance",
		params: []any{account, toBlockNumArg(blockNumber)},
	}
}

func (p *Provider) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	var ret *big.Int
	err := p.Do(ctx, BalanceAt(account, blockNumber).Into(&ret))
	return ret, err
}

func SendTransaction(tx *types.Transaction) Call {
	data, err := tx.MarshalBinary()
	if err != nil {
		return Call{err: err}
	}
	return Call{
		request:  makeMessage("eth_sendRawTransaction", []any{hexutil.Encode(data)}),
		resultFn: nil,
	}
}

func (p *Provider) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return p.Do(ctx, SendTransaction(tx))
}

func BlockByHash(hash common.Hash) CallBuilder[*types.Block] {
	return CallBuilder[*types.Block]{
		method: "eth_getBlockByHash",
		params: []any{hash},
		intoFn: intoBlock,
	}
}

func (p *Provider) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	var ret *types.Block
	err := p.Do(ctx, BlockByHash(hash).Into(&ret))
	return ret, err
}

func BlockByNumber(number *big.Int) CallBuilder[*types.Block] {
	return CallBuilder[*types.Block]{
		method: "eth_getBlockByNumber",
		params: []any{toBlockNumArg(number)},
		intoFn: intoBlock,
	}
}

func (p *Provider) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	var ret *types.Block
	err := p.Do(ctx, BlockByNumber(number).Into(&ret))
	return ret, err
}

func PeerCount() CallBuilder[uint64] {
	return CallBuilder[uint64]{
		method: "net_peerCount",
		intoFn: hexIntoUint64,
	}
}

func (p *Provider) PeerCount(ctx context.Context) (uint64, error) {
	var ret uint64
	err := p.Do(ctx, PeerCount().Into(&ret))
	return ret, err
}

func HeaderByHash(hash common.Hash) CallBuilder[*types.Header] {
	return CallBuilder[*types.Header]{
		method: "eth_getBlockByHash",
		params: []any{hash},
	}
}

func (p *Provider) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	var head *types.Header
	err := p.Do(ctx, HeaderByHash(hash).Into(&head))
	return head, err
}

func HeaderByNumber(number *big.Int) CallBuilder[*types.Header] {
	return CallBuilder[*types.Header]{
		method: "eth_getBlockByNumber",
		params: []any{toBlockNumArg(number)},
	}
}

func (p *Provider) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	var head *types.Header
	err := p.Do(ctx, HeaderByNumber(number).Into(&head))
	return head, err
}

func (p *Provider) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, pending bool, err error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) TransactionSender(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) NetworkID(ctx context.Context) (*big.Int, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) PendingTransactionCount(ctx context.Context) (uint, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) PendingCallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Provider) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	//TODO implement me
	panic("implement me")
}
