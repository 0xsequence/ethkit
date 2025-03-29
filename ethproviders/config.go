package ethproviders

import "strings"

type Config map[string]NetworkConfig

type NetworkConfig struct {
	// ID is the globally unique chain ID. See https://chainlist.wtf
	ID uint64 `toml:"id" json:"id"`

	// URL is the URL for the blockchain node JSON-RPC endpoint
	URL string `toml:"url" json:"url"`

	// WSEnabled marks the chain to support websocket connections.
	// NOTE: you may leave `WSURL` empty and it will use the `URL`
	// of the node as the WSURL by default. Or you can set `WSURL` to
	// another URL for websocket connections
	WSEnabled bool `toml:"ws_enabled" json:"wsEnabled"`

	// WSURL is the URL for the websocket. You must also set `WSEnabled`
	// to `true`
	WSURL string `toml:"ws_url" json:"wsUrl"`

	// Testnet marks the chain as a testnet.
	Testnet bool `toml:"testnet" json:"testnet"`

	// AuthChain marks the chain as an auth chain.
	// Deprecated: no longer required.
	AuthChain bool `toml:"auth_chain" json:"authChain"`

	// Disabled marks the chain as disabled, and will not be included
	// in the list of providers at runtime.
	Disabled bool `toml:"disabled" json:"disabled"`
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

// Deprecated: no longer required.
func (n Config) AuthChain() (NetworkConfig, bool) {
	for _, v := range n {
		if v.AuthChain && !v.Testnet {
			return v, true
		}
	}
	return NetworkConfig{}, false
}

// Deprecated: no longer required.
func (n Config) TestAuthChain() (NetworkConfig, bool) {
	for _, v := range n {
		if v.AuthChain && v.Testnet {
			return v, true
		}
	}
	return NetworkConfig{}, false
}
