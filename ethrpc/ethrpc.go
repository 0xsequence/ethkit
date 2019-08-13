package ethrpc

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
)

// the goal should be a simple ethclient with round-robin support and perhaps a few extra methods.

// TODO: can make a pool of ethclient.Client
// first check them all to ensure they're of the same chainid and syncing is in right state
// ..
// as well, add timeout checking in case one goes down / doesn't respond, to keep it out of the pool for some time..
// etc.

type JSONRPC struct {
	*ethclient.Client
}

var _ bind.ContractBackend = &JSONRPC{}

// TODO: maybe keep this as simple as passing a bunch of ethclient.Client objects..?
func NewJSONRPC(ethURL string) (*JSONRPC, error) {
	client, err := ethclient.Dial(ethURL)
	if err != nil {
		return nil, err
	}

	return &JSONRPC{
		Client: client,
	}, nil
}
