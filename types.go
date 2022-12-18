package ethkit

import "github.com/0xsequence/ethkit/go-ethereum/common"

type Address = common.Address

type Hash = common.Hash

const HashLength = common.HashLength

func PtrTo[T any](v T) *T {
	return &v
}
