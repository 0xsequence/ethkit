package ethproviders

import "strings"

type Config map[string]NetworkConfig

type NetworkConfig struct {
	ID        uint64 `toml:"id" json:"id"`
	URL       string `toml:"url" json:"url"`
	AuthChain bool   `toml:"auth_chain" json:"authChain"`
	Testnet   bool   `toml:"testnet" json:"testnet"`
	Disabled  bool   `toml:"disabled" json:"disabled"`
}

func (n Config) GetByID(id uint64) (NetworkConfig, bool) {
	for _, v := range n {
		if v.ID == id {
			return v, true
		}
	}
	return NetworkConfig{}, false
}

func (n Config) GetByName(name string) (NetworkConfig, bool) {
	name = strings.ToLower(name)
	for k, v := range n {
		if k == name {
			return v, true
		}
	}
	return NetworkConfig{}, false
}

func (n Config) AuthChain() (NetworkConfig, bool) {
	for _, v := range n {
		if v.AuthChain && !v.Testnet {
			return v, true
		}
	}
	return NetworkConfig{}, false
}

func (n Config) TestAuthChain() (NetworkConfig, bool) {
	for _, v := range n {
		if v.AuthChain && v.Testnet {
			return v, true
		}
	}
	return NetworkConfig{}, false
}
