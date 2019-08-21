package ethrpc

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
)

type JSONRPC struct {
	*ethclient.Client
	Config *Config
}

var _ bind.ContractBackend = &JSONRPC{}

// for the batch client, the challenge will be to make sure all nodes are syncing to the same beat

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
