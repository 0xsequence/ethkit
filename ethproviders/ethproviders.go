package ethproviders

import (
	"fmt"
	"sort"

	"github.com/0xsequence/ethkit/ethrpc"
)

type Providers struct {
	byID          map[uint64]*ethrpc.Provider
	byName        map[string]*ethrpc.Provider
	configByID    map[uint64]NetworkConfig
	authChain     *ethrpc.Provider
	testAuthChain *ethrpc.Provider
	chainList     []ChainInfo
}

type ChainInfo struct {
	ID   uint64 `json:"id"` // TODO: switch to *big.Int
	Name string `json:"name"`
}

func NewProviders(cfg Config, opts ...ethrpc.Option) (*Providers, error) {
	providers := &Providers{
		byID:       map[uint64]*ethrpc.Provider{},
		byName:     map[string]*ethrpc.Provider{},
		configByID: map[uint64]NetworkConfig{},
	}

	for name, details := range cfg {
		if details.Disabled {
			continue
		}

		p, err := ethrpc.NewProvider(details.URL, opts...)
		if err != nil {
			return nil, err
		}
		providers.byID[details.ID] = p
		providers.byName[name] = p
		providers.configByID[details.ID] = details

		if (details.AuthChain && !details.Testnet && providers.authChain != nil) || (details.AuthChain && details.Testnet && providers.testAuthChain != nil) {
			return nil, fmt.Errorf("duplicate auth chain providers detected in config")
		}
		if details.AuthChain && !details.Testnet {
			providers.authChain = p
		}
		if details.AuthChain && details.Testnet {
			providers.testAuthChain = p
		}
	}
	if len(providers.byID) != len(providers.byName) {
		return nil, fmt.Errorf("duplicate provider id or name detected")
	}

	// also record the chain number as string for easier lookup
	for k, p := range providers.byID {
		providers.byName[fmt.Sprintf("%d", k)] = p
	}

	// build the chain list object
	chainList := []ChainInfo{}
	for name, networkConfig := range cfg {
		if networkConfig.Disabled {
			continue
		}

		chainList = append(chainList, ChainInfo{ID: networkConfig.ID, Name: name})
	}
	sort.SliceStable(chainList, func(i, j int) bool {
		return chainList[i].ID < chainList[j].ID
	})
	providers.chainList = chainList

	return providers, nil
}

// Get is a helper method which will allow you to fetch the provider for a chain by either
// the chain canonical name, or by the chain canonical id. This works because at the time
// of configuring the providers list in `NewProviders` we assign the name and string-id
// to the byName mapping.
func (p *Providers) Get(chainHandle string) *ethrpc.Provider {
	return p.byName[chainHandle]
}

func (p *Providers) GetByChainID(chainID uint64) *ethrpc.Provider {
	return p.byID[chainID]
}

func (p *Providers) GetByChainName(chainName string) *ethrpc.Provider {
	return p.byName[chainName]
}

func (p *Providers) GetAuthChain() *ethrpc.Provider {
	return p.authChain
}

func (p *Providers) GetTestAuthChain() *ethrpc.Provider {
	return p.testAuthChain
}

func (p *Providers) GetAuthProvider() *ethrpc.Provider {
	return p.authChain
}

func (p *Providers) GetTestnetAuthProvider() *ethrpc.Provider {
	return p.testAuthChain
}

func (p *Providers) ProviderMap() map[uint64]*ethrpc.Provider {
	return p.byID
}

func (p *Providers) LookupAuthProviderByChainID(chainID uint64) *ethrpc.Provider {
	details, ok := p.configByID[chainID]
	if !ok {
		return nil
	}
	if details.Testnet {
		return p.testAuthChain
	} else {
		return p.authChain
	}
}

func (p *Providers) ChainList() []ChainInfo {
	return p.chainList
}

func (p *Providers) FindChain(chainHandle string) (uint64, ChainInfo, error) {
	for _, info := range p.chainList {
		if chainHandle == info.Name || chainHandle == fmt.Sprintf("%d", info.ID) {
			return info.ID, info, nil // found
		}
	}
	return 0, ChainInfo{}, fmt.Errorf("chainID not found")
}
