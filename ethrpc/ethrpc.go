package ethrpc

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Provider struct {
	*ethclient.Client
	Config *Config
}

var _ bind.ContractBackend = &Provider{}

// for the batch client, the challenge will be to make sure all nodes are
// syncing to the same beat

func NewProvider(ethURL string) (*Provider, error) {
	config := DefaultProviderConfig
	config.AddNode(NodeConfig{URL: ethURL})
	return NewProviderWithConfig(config)
}

func NewProviderWithConfig(config *Config) (*Provider, error) {
	provider := &Provider{
		Config: config,
	}
	err := provider.Dial()
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func (s *Provider) Dial() error {
	// TODO: batch client support
	client, err := ethclient.Dial(s.Config.Nodes[0].URL)
	if err != nil {
		return err
	}
	s.Client = client
	return nil
}
