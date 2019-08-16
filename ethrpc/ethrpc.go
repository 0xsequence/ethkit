package ethrpc

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
)

type JSONRPC struct {
	*ethclient.Client
	Config *Config
}

var _ bind.ContractBackend = &JSONRPC{}

func NewJSONRPC(ethURL string) (*JSONRPC, error) {
	config := DefaultJSONRPCConfig
	config.AddNode(NodeConfig{URL: ethURL})
	return NewJSONRPCWithConfig(config)
}

func NewJSONRPCWithConfig(config *Config) (*JSONRPC, error) {
	provider := &JSONRPC{
		Config: config,
	}
	err := provider.Dial()
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func (s *JSONRPC) Dial() error {
	// TODO: later..
	client, err := ethclient.Dial(s.Config.Nodes[0].URL)
	if err != nil {
		return err
	}
	s.Client = client
	return nil
}

func (s *JSONRPC) WaitForTxnReceipt(ctx context.Context, txnHash common.Hash, timeout time.Duration) (*types.Receipt, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		receipt, err := s.Client.TransactionReceipt(timeoutCtx, txnHash)
		if err != nil && err != ethereum.NotFound {
			return nil, err
		}
		if receipt != nil {
			return receipt, nil
		}
		if receipt == nil && s.Config.BlockTime > 0 {
			time.Sleep(s.Config.BlockTime)
			continue
		}
		return nil, errors.Errorf("receipt not found")
	}
}
