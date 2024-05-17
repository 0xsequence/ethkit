package ethrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync/atomic"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/rpc"
	"github.com/goware/breaker"
	"github.com/goware/logger"
	"github.com/goware/superr"
)

type Provider struct {
	log        logger.Logger
	nodeURL    string
	nodeWSURL  string
	httpClient httpClient
	br         breaker.Breaker
	jwtToken   string // optional

	chainID *big.Int
	// cache   cachestore.Store[[]byte] // NOTE: unused for now
	lastRequestID uint64

	gethRPC *rpc.Client
}

func NewProvider(nodeURL string, options ...Option) (*Provider, error) {
	p := &Provider{
		nodeURL:    nodeURL,
		httpClient: http.DefaultClient,
	}
	for _, opt := range options {
		if opt == nil {
			continue
		}
		opt(p)
	}
	return p, nil
}

var (
	ErrNotFound                 = ethereum.NotFound
	ErrEmptyResponse            = errors.New("ethrpc: empty response")
	ErrUnsupportedMethodOnChain = errors.New("ethrpc: method is unsupported on this chain")
	ErrRequestFail              = errors.New("ethrpc: request fail")
)

var _ Interface = &Provider{}
var _ RawInterface = &Provider{}

// Provider adheres to the go-ethereum bind.ContractBackend interface. In case we ever
// want to break this interface, we could also write an adapter type to keep them compat.
var _ bind.ContractBackend = &Provider{}

func (s *Provider) SetHTTPClient(httpClient *http.Client) {
	s.httpClient = httpClient
}

func (p *Provider) Do(ctx context.Context, calls ...Call) ([]byte, error) {
	if len(calls) == 0 {
		return nil, nil
	}

	batch := make(BatchCall, 0, len(calls))
	for i, call := range calls {
		call := call
		if call.err != nil {
			// TODO: store and return the error but execute the rest of the batch?
			return nil, fmt.Errorf("call %d has an error: %w", i, call.err)
		}

		call.request.ID = atomic.AddUint64(&p.lastRequestID, 1)
		batch = append(batch, &call)
	}

	b, err := batch.MarshalJSON()
	if err != nil {
		return nil, superr.Wrap(ErrRequestFail, fmt.Errorf("failed to marshal JSONRPC request: %w", err))
	}

	req, err := http.NewRequest(http.MethodPost, p.nodeURL, bytes.NewBuffer(b))
	if err != nil {
		return nil, superr.Wrap(ErrRequestFail, fmt.Errorf("failed to initialize http.Request: %w", err))
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	if p.jwtToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("BEARER %s", p.jwtToken))
	}

	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, superr.Wrap(ErrRequestFail, fmt.Errorf("failed to send request: %w", err))
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, superr.Wrap(ErrRequestFail, fmt.Errorf("failed to read resposne body: %w", err))
	}

	if (res.StatusCode < 200 || res.StatusCode > 299) && res.StatusCode != 401 {
		if len(body) > 100 {
			body = body[:100]
		}
		return body, superr.Wrap(ErrRequestFail, fmt.Errorf("non-200 response with status code: %d with body '%s'", res.StatusCode, body))
	}

	if err := json.Unmarshal(body, &batch); err != nil {
		if len(body) > 100 {
			body = body[:100]
		}
		return body, superr.Wrap(ErrRequestFail, fmt.Errorf("failed to unmarshal response: '%s' due to %w", string(body), err))
	}

	for i, call := range batch {
		if call.err != nil {
			continue
		}

		// no response
		if call.response == nil {
			call.err = ErrEmptyResponse
			continue
		}

		// ensure response id matches the request id, otherwise
		// the node is doing something wonky.
		if call.request.ID != call.response.ID {
			call.err = superr.Wrap(ErrRequestFail, fmt.Errorf("response id (%d) does not match request id (%d)", call.response.ID, call.request.ID))
			continue
		}

		// expecting no result, so we skip
		if calls[i].resultFn == nil {
			continue
		}

		if err := calls[i].resultFn(call.response.Result); err != nil {
			call.err = err
			continue
		}
	}

	return body, batch.ErrorOrNil()
}

func (p *Provider) ChainID(ctx context.Context) (*big.Int, error) {
	if p.chainID != nil {
		// chainID is memoized
		return p.chainID, nil
	}
	var ret *big.Int
	_, err := p.Do(ctx, ChainID().Into(&ret))
	if err != nil {
		return nil, err
	}
	p.chainID = ret
	return ret, nil
}

func (p *Provider) BlockNumber(ctx context.Context) (uint64, error) {
	var ret uint64
	_, err := p.Do(ctx, BlockNumber().Into(&ret))
	return ret, err
}

func (p *Provider) BalanceAt(ctx context.Context, account common.Address, blockNum *big.Int) (*big.Int, error) {
	var ret *big.Int
	_, err := p.Do(ctx, BalanceAt(account, blockNum).Into(&ret))
	return ret, err
}

func (p *Provider) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	_, err := p.Do(ctx, SendTransaction(tx))
	return err
}

func (s *Provider) SendRawTransaction(ctx context.Context, signedTxHex string) (common.Hash, error) {
	var txnHash common.Hash
	_, err := s.Do(ctx, SendRawTransaction(signedTxHex).Into(&txnHash))
	return txnHash, err
}

func (p *Provider) RawBlockByHash(ctx context.Context, hash common.Hash) (json.RawMessage, error) {
	var result json.RawMessage
	_, err := p.Do(ctx, RawBlockByHash(hash).Into(&result))
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, ethereum.NotFound
	}
	return result, nil
}

func (p *Provider) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	var ret *types.Block
	_, err := p.Do(ctx, BlockByHash(hash).Into(&ret))
	return ret, err
}

func (p *Provider) RawBlockByNumber(ctx context.Context, blockNum *big.Int) (json.RawMessage, error) {
	var result json.RawMessage
	_, err := p.Do(ctx, RawBlockByNumber(blockNum).Into(&result))
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, ethereum.NotFound
	}
	return result, nil
}

func (p *Provider) BlockByNumber(ctx context.Context, blockNum *big.Int) (*types.Block, error) {
	var ret *types.Block
	_, err := p.Do(ctx, BlockByNumber(blockNum).Into(&ret))
	return ret, err
}

func (p *Provider) PeerCount(ctx context.Context) (uint64, error) {
	var ret uint64
	_, err := p.Do(ctx, PeerCount().Into(&ret))
	return ret, err
}

func (p *Provider) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	var head *types.Header
	_, err := p.Do(ctx, HeaderByHash(hash).Into(&head))
	if err == nil && head == nil {
		return nil, ethereum.NotFound
	}
	return head, err
}

func (p *Provider) HeaderByNumber(ctx context.Context, blockNum *big.Int) (*types.Header, error) {
	var head *types.Header
	_, err := p.Do(ctx, HeaderByNumber(blockNum).Into(&head))
	if err == nil && head == nil {
		return nil, ethereum.NotFound
	}
	return head, err
}

func (p *Provider) HeadersByNumbers(ctx context.Context, blockNumbers []*big.Int) ([]*types.Header, error) {
	var headers = make([]*types.Header, len(blockNumbers))

	var calls []Call
	for index, blockNum := range blockNumbers {
		calls = append(calls, HeaderByNumber(blockNum).Into(&headers[index]))
	}

	_, err := p.Do(ctx, calls...)
	return headers, err
}

func (p *Provider) HeadersByNumberRange(ctx context.Context, fromBlockNumber, toBlockNumber *big.Int) ([]*types.Header, error) {
	var blockNumbers []*big.Int
	for i := big.NewInt(0).Set(fromBlockNumber); i.Cmp(toBlockNumber) < 0; i.Add(i, big.NewInt(1)) {
		blockNumbers = append(blockNumbers, big.NewInt(0).Set(i))
	}
	return p.HeadersByNumbers(ctx, blockNumbers)
}

func (p *Provider) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, pending bool, err error) {
	_, err = p.Do(ctx, TransactionByHash(hash).Into(&tx, &pending))
	if err == nil && tx == nil {
		return nil, false, ethereum.NotFound
	}
	return tx, pending, err
}

func (p *Provider) TransactionSender(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error) {
	sender, err := types.Sender(&senderFromServer{blockhash: block}, tx)
	if err != nil {
		return sender, nil
	}
	_, err = p.Do(ctx, TransactionSender(tx, block, index).Into(&sender))
	return sender, err
}

func (p *Provider) TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error) {
	var ret uint
	_, err := p.Do(ctx, TransactionCount(blockHash).Into(&ret))
	return ret, err
}

func (p *Provider) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
	var tx *types.Transaction
	_, err := p.Do(ctx, TransactionInBlock(blockHash, index).Into(&tx))
	if err == nil && tx == nil {
		return nil, ethereum.NotFound
	}
	return tx, err
}

func (p *Provider) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	_, err := p.Do(ctx, TransactionReceipt(txHash).Into(&receipt))
	if err == nil && receipt == nil {
		return nil, ethereum.NotFound
	}
	return receipt, err
}

func (p *Provider) SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error) {
	var progress *ethereum.SyncProgress
	_, err := p.Do(ctx, SyncProgress().Into(&progress))
	return progress, err
}

func (p *Provider) NetworkID(ctx context.Context) (*big.Int, error) {
	var version *big.Int
	_, err := p.Do(ctx, NetworkID().Into(&version))
	return version, err
}

func (p *Provider) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNum *big.Int) ([]byte, error) {
	var result []byte
	_, err := p.Do(ctx, StorageAt(account, key, blockNum).Into(&result))
	return result, err
}

func (p *Provider) CodeAt(ctx context.Context, account common.Address, blockNum *big.Int) ([]byte, error) {
	var result []byte
	_, err := p.Do(ctx, CodeAt(account, blockNum).Into(&result))
	return result, err
}

func (p *Provider) NonceAt(ctx context.Context, account common.Address, blockNum *big.Int) (uint64, error) {
	var result uint64
	_, err := p.Do(ctx, NonceAt(account, blockNum).Into(&result))
	return result, err
}

func (p *Provider) RawFilterLogs(ctx context.Context, q ethereum.FilterQuery) (json.RawMessage, error) {
	var result json.RawMessage
	_, err := p.Do(ctx, RawFilterLogs(q).Into(&result))
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	var logs []types.Log
	_, err := p.Do(ctx, FilterLogs(q).Into(&logs))
	return logs, err
}

func (p *Provider) PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	var ret *big.Int
	_, err := p.Do(ctx, PendingBalanceAt(account).Into(&ret))
	return ret, err
}

func (p *Provider) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
	var result []byte
	_, err := p.Do(ctx, PendingStorageAt(account, key).Into(&result))
	return result, err
}

func (p *Provider) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	var result []byte
	_, err := p.Do(ctx, PendingCodeAt(account).Into(&result))
	return result, err
}

func (p *Provider) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	var result uint64
	_, err := p.Do(ctx, PendingNonceAt(account).Into(&result))
	return result, err
}

func (p *Provider) PendingTransactionCount(ctx context.Context) (uint, error) {
	var ret uint
	_, err := p.Do(ctx, PendingTransactionCount().Into(&ret))
	return ret, err
}

func (p *Provider) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNum *big.Int) ([]byte, error) {
	var result []byte
	_, err := p.Do(ctx, CallContract(msg, blockNum).Into(&result))
	return result, err
}

func (p *Provider) CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error) {
	var result []byte
	_, err := p.Do(ctx, CallContractAtHash(msg, blockHash).Into(&result))
	return result, err
}

func (p *Provider) PendingCallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	var result []byte
	_, err := p.Do(ctx, PendingCallContract(msg).Into(&result))
	return result, err
}

func (p *Provider) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	var ret *big.Int
	_, err := p.Do(ctx, SuggestGasPrice().Into(&ret))
	return ret, err
}

func (p *Provider) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	var ret *big.Int
	_, err := p.Do(ctx, SuggestGasTipCap().Into(&ret))
	return ret, err
}

func (p *Provider) FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
	var fh *ethereum.FeeHistory
	_, err := p.Do(ctx, FeeHistory(blockCount, lastBlock, rewardPercentiles).Into(&fh))
	return fh, err
}

func (p *Provider) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	var result uint64
	_, err := p.Do(ctx, EstimateGas(msg).Into(&result))
	return result, err
}

// SubscribeFilterLogs is stubbed below so we can adhere to the bind.ContractBackend interface.
func (p *Provider) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	if !p.IsStreamingEnabled() {
		return nil, fmt.Errorf("ethrpc: provider instance has not enabled streaming")
	}
	if p.gethRPC == nil {
		var err error
		p.gethRPC, err = rpc.Dial(p.nodeWSURL)
		if err != nil {
			return nil, fmt.Errorf("ethrpc: SubscribeFilterLogs failed: %w", err)
		}
	}

	return p.gethRPC.EthSubscribe(ctx, ch, "logs", query)
}

// ..
func (p *Provider) SubscribeNewHeads(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	if !p.IsStreamingEnabled() {
		return nil, fmt.Errorf("ethrpc: provider instance has not enabled streaming")
	}
	if p.gethRPC == nil {
		var err error
		p.gethRPC, err = rpc.Dial(p.nodeWSURL)
		if err != nil {
			return nil, fmt.Errorf("ethrpc: SubscribeNewHeads failed: %w", err)
		}
	}

	return p.gethRPC.EthSubscribe(ctx, ch, "newHeads")
}

// ie, ContractQuery(context.Background(), "0xabcdef..", "balanceOf(uint256)", "uint256", []string{"1"})
// TODO: add common methods in helpers util, and also use generics to convert the return for us
func (p *Provider) ContractQuery(ctx context.Context, contractAddress string, inputAbiExpr, outputAbiExpr string, args interface{}) ([]string, error) {
	if !common.IsHexAddress(contractAddress) {
		// Check for ens
		ensAddress, ok, err := ResolveEnsAddress(ctx, contractAddress, p)
		if err != nil {
			return nil, fmt.Errorf("ethrpc: contract address is not a valid address or an ens domain %w", err)
		}
		if ok {
			contractAddress = ensAddress.Hex()
		}
	}

	return p.contractQuery(ctx, contractAddress, inputAbiExpr, outputAbiExpr, args)
}

func (p *Provider) contractQuery(ctx context.Context, contractAddress string, inputAbiExpr, outputAbiExpr string, args interface{}) ([]string, error) {
	contract := common.HexToAddress(contractAddress)

	contractQueryBuilder, err := ContractQuery(contract, inputAbiExpr, outputAbiExpr, args)
	if err != nil {
		return nil, err
	}

	var result []string
	_, err = p.Do(ctx, contractQueryBuilder.Into(&result))
	return result, err
}

// ...
func (p *Provider) IsStreamingEnabled() bool {
	return p.nodeWSURL != ""
}
