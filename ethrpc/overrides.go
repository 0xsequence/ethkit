package ethrpc

import "github.com/0xsequence/ethkit/go-ethereum/ethclient/gethclient"

// OverrideAccount specifies the state of an account to be overridden.
// This is an alias of gethclient.OverrideAccount so callers don't need to import gethclient.
type OverrideAccount = gethclient.OverrideAccount
