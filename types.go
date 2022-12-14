package ethkit

import "github.com/0xsequence/ethkit/go-ethereum/common"

type Address = common.Address

type Hash = common.Hash

func PtrTo[T any](v T) *T {
	return &v
}
