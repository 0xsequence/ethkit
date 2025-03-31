package ethproviders

import (
	"fmt"
	"sort"

	"github.com/0xsequence/ethkit/ethrpc"
)

type Providers struct {
	byID       map[uint64]*ethrpc.Provider
	byName     map[string]*ethrpc.Provider
	configByID map[uint64]NetworkConfig

	allChainsList []ChainInfo
	mainnetsList  []ChainInfo
	testnetsList  []ChainInfo

	mainnetChainIDs       []uint64
	testnetChainIDs       []uint64
	mainnetStringChainIDs []string
	testnetStringChainIDs []string

	// Deprecated
	// TODO: remove in the future
	authChain     *ethrpc.Provider
	testAuthChain *ethrpc.Provider
}

type ChainInfo struct {
	// ID is the globally unique chain ID. See https://chainlist.wtf
	ID uint64 `json:"id"`

	// Name is the canonical name of the chain. See https://docs.sequence.xyz
	Name string `json:"name"`

	// Testnet is true if the chain is a testnet.
	Testnet bool `json:"testnet"`
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
		chainList = append(chainList, ChainInfo{ID: networkConfig.ID, Name: name, Testnet: networkConfig.Testnet})
	}
	sort.SliceStable(chainList, func(i, j int) bool {
		return chainList[i].ID < chainList[j].ID
	})
	providers.allChainsList = chainList

	for _, chain := range chainList {
		if chain.Testnet {
			providers.testnetsList = append(providers.testnetsList, chain)
			providers.testnetChainIDs = append(providers.testnetChainIDs, chain.ID)
			providers.testnetStringChainIDs = append(providers.testnetStringChainIDs, fmt.Sprintf("%d", chain.ID))
		} else {
			providers.mainnetsList = append(providers.mainnetsList, chain)
			providers.mainnetChainIDs = append(providers.mainnetChainIDs, chain.ID)
			providers.mainnetStringChainIDs = append(providers.mainnetStringChainIDs, fmt.Sprintf("%d", chain.ID))
		}
	}

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
	return p.allChainsList
}

func (p *Providers) MainnetChainList() []ChainInfo {
	return p.mainnetsList
}

func (p *Providers) TestnetChainList() []ChainInfo {
	return p.testnetsList
}

func (p *Providers) MainnetChainIDs() []uint64 {
	return p.mainnetChainIDs
}

func (p *Providers) TestnetChainIDs() []uint64 {
	return p.testnetChainIDs
}

func (p *Providers) MainnetStringChainIDs() []string {
	return p.mainnetStringChainIDs
}

func (p *Providers) TestnetStringChainIDs() []string {
	return p.testnetStringChainIDs
}

func (p *Providers) FindChain(chainHandle string, optSkipTestnets ...bool) (uint64, ChainInfo, error) {
	chainList := p.allChainsList
	if len(optSkipTestnets) > 0 && optSkipTestnets[0] {
		chainList = p.mainnetsList
	}
	for _, info := range chainList {
		if chainHandle == info.Name || chainHandle == fmt.Sprintf("%d", info.ID) {
			return info.ID, info, nil // found
		}
	}
	return 0, ChainInfo{}, fmt.Errorf("chainID not found")
}
