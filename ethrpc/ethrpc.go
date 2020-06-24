package ethrpc

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type Provider struct {
	*ethclient.Client
	Config     *Config
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

	return nil
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
