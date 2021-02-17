package ethrpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// NOTE: most of the code in the current implementatio is from go-ethereum and been
// modified for ease of use. Future implementations of this package will forego use of
// go-ethereum code or as dependency.

type Provider struct {
	*ethclient.Client
	Config     *Config
	RPC        *rpc.Client
	httpClient *http.Client
}

var _ bind.ContractBackend = &Provider{}

// for the batch client, the challenge will be to make sure all nodes are
// syncing to the same beat

func NewProvider(ethURL string, optClient ...*http.Client) (*Provider, error) {
	if ethURL == "" {
		return nil, errors.New("ethrpc: provider url cannot be empty.")
	}

	config := &Config{}
	config.AddNode(NodeConfig{URL: ethURL})
	return NewProviderWithConfig(config, optClient...)
}

func NewProviderWithConfig(config *Config, optClient ...*http.Client) (*Provider, error) {
	provider := &Provider{
		Config: config,
	}
	if len(optClient) > 0 {
		provider.httpClient = optClient[0]
	}

	err := provider.Dial()
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func (s *Provider) Dial() error {
	// TODO: batch client support
	url := s.Config.Nodes[0].URL

	var rpcClient *rpc.Client
	var err error

	if s.httpClient != nil {
		rpcClient, err = rpc.DialHTTPWithClient(url, s.httpClient)
	} else {
		rpcClient, err = rpc.DialHTTP(url)
	}
	if err != nil {
		return err
	}

	s.Client = ethclient.NewClient(rpcClient)
	s.RPC = rpcClient

	return nil
}

func (s *Provider) ChainID(ctx context.Context) (*big.Int, error) {
	// When querying a local node, we expect the server to be ganache, which will always return chainID of 1337
	// for eth_chainId call, so instead call net_version method instead for the correct value. Wth.
	nodeURL := s.Config.Nodes[0].URL
	if strings.Index(nodeURL, "0.0.0.0") > 0 || strings.Index(nodeURL, "localhost") > 0 || strings.Index(nodeURL, "127.0.0.1") > 0 {
		return s.Client.NetworkID(ctx)
	}

	// call eth_chainId for non-local node calls
	return s.Client.ChainID(ctx)
}

func (s *Provider) TransactionDetails(ctx context.Context, txnHash common.Hash) (bool, *types.Receipt, *types.Transaction, string, error) {
	receipt, err := s.TransactionReceipt(ctx, txnHash)
	if err != nil {
		return false, nil, nil, "", err
	}

	status := receipt.Status == types.ReceiptStatusSuccessful

	txn, _, err := s.TransactionByHash(ctx, txnHash)
	if err != nil {
		return status, receipt, txn, "", err
	}

	if receipt.GasUsed == txn.Gas() {
		return status, receipt, txn, "OUT OF GAS", nil
	}

	txnMsg, err := txn.AsMessage(types.NewEIP155Signer(txn.ChainId()))
	if err != nil {
		return status, receipt, txn, "", err
	}

	callMsg := ethereum.CallMsg{
		From:     txnMsg.From(),
		To:       txn.To(),
		Gas:      txn.Gas(),
		GasPrice: txn.GasPrice(),
		Value:    txn.Value(),
		Data:     txn.Data(),
	}

	raw, err := s.CallContract(context.Background(), callMsg, receipt.BlockNumber)
	if err != nil {
		return status, receipt, txn, "", err
	}

	rawHex := hexutil.Encode(raw)
	rawMessageData := rawHex[2:]
	strLenHex := rawMessageData[8+64 : 8+128]
	strLen, err := strconv.ParseInt(strLenHex, 16, 64)
	if err != nil {
		return status, receipt, txn, "", err
	}

	revertReasonHex := rawMessageData[8+128 : 8+128+(strLen*2)]
	revertReason, err := hex.DecodeString(revertReasonHex)
	if err != nil {
		return status, receipt, txn, "", err
	}

	return status, receipt, txn, string(revertReason), nil
}

func (s *Provider) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return s.getBlock2(ctx, "eth_getBlockByHash", hash, true)
}

func (s *Provider) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return s.getBlock2(ctx, "eth_getBlockByNumber", toBlockNumArg(number), true)
}

type rpcTransaction struct {
	tx *types.Transaction
	txExtraInfo
}

func (tx *rpcTransaction) UnmarshalJSON(msg []byte) error {
	if err := json.Unmarshal(msg, &tx.tx); err != nil {
		return err
	}
	return json.Unmarshal(msg, &tx.txExtraInfo)
}

type rpcBlock struct {
	Hash         common.Hash      `json:"hash"`
	Transactions []rpcTransaction `json:"transactions"`
	UncleHashes  []common.Hash    `json:"uncles"`
}

type txExtraInfo struct {
	BlockNumber *string         `json:"blockNumber,omitempty"`
	BlockHash   *common.Hash    `json:"blockHash,omitempty"`
	From        *common.Address `json:"from,omitempty"`
}

func (s *Provider) getBlock2(ctx context.Context, method string, args ...interface{}) (*types.Block, error) {
	var raw json.RawMessage
	err := s.RPC.CallContext(ctx, &raw, method, args...)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, ethereum.NotFound
	}
	// Decode header and transactions.
	var head *types.Header
	var body rpcBlock
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}

	// Quick-verify transaction and uncle lists. This mostly helps with debugging the server.
	// if head.UncleHash == types.EmptyUncleHash && len(body.UncleHashes) > 0 {
	// 	return nil, fmt.Errorf("server returned non-empty uncle list but block header indicates no uncles")
	// }
	// if head.UncleHash != types.EmptyUncleHash && len(body.UncleHashes) == 0 {
	// 	return nil, fmt.Errorf("server returned empty uncle list but block header indicates uncles")
	// }
	// if head.TxHash == types.EmptyRootHash && len(body.Transactions) > 0 {
	// 	return nil, fmt.Errorf("server returned non-empty transaction list but block header indicates no transactions")
	// }
	// if head.TxHash != types.EmptyRootHash && len(body.Transactions) == 0 {
	// 	return nil, fmt.Errorf("server returned empty transaction list but block header indicates transactions")
	// }

	// Load uncles because they are not included in the block response.
	var uncles []*types.Header
	if len(body.UncleHashes) > 0 {
		uncles = make([]*types.Header, len(body.UncleHashes))
		reqs := make([]rpc.BatchElem, len(body.UncleHashes))
		for i := range reqs {
			reqs[i] = rpc.BatchElem{
				Method: "eth_getUncleByBlockHashAndIndex",
				Args:   []interface{}{body.Hash, hexutil.EncodeUint64(uint64(i))},
				Result: &uncles[i],
			}
		}
		if err := s.RPC.BatchCallContext(ctx, reqs); err != nil {
			return nil, err
		}
		for i := range reqs {
			if reqs[i].Error != nil {
				return nil, reqs[i].Error
			}
			if uncles[i] == nil {
				return nil, fmt.Errorf("got null header for uncle %d of block %x", i, body.Hash[:])
			}
		}
	}
	// Fill the sender cache of transactions in the block.
	txs := make([]*types.Transaction, len(body.Transactions))
	for i, tx := range body.Transactions {
		if tx.From != nil {
			setSenderFromServer(tx.tx, *tx.From, body.Hash)
		}
		txs[i] = tx.tx
	}

	return types.NewBlockWithHeader(head).WithBody(txs, uncles), nil
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	return hexutil.EncodeBig(number)
}

// senderFromServer is a types.Signer that remembers the sender address returned by the RPC
// server. It is stored in the transaction's sender address cache to avoid an additional
// request in TransactionSender.
type senderFromServer struct {
	addr      common.Address
	blockhash common.Hash
}

var errNotCached = errors.New("sender not cached")

func setSenderFromServer(tx *types.Transaction, addr common.Address, block common.Hash) {
	// Use types.Sender for side-effect to store our signer into the cache.
	types.Sender(&senderFromServer{addr, block}, tx)
}

func (s *senderFromServer) Equal(other types.Signer) bool {
	os, ok := other.(*senderFromServer)
	return ok && os.blockhash == s.blockhash
}

func (s *senderFromServer) Sender(tx *types.Transaction) (common.Address, error) {
	if s.blockhash == (common.Hash{}) {
		return common.Address{}, errNotCached
	}
	return s.addr, nil
}

func (s *senderFromServer) Hash(tx *types.Transaction) common.Hash {
	panic("can't sign with senderFromServer")
}
func (s *senderFromServer) SignatureValues(tx *types.Transaction, sig []byte) (R, S, V *big.Int, err error) {
	panic("can't sign with senderFromServer")
}
