package ethkit

import "github.com/0xsequence/ethkit/go-ethereum/common"

type Address = common.Address

type Hash = common.Hash

const HashLength = common.HashLength

func ToPtr[T any](v T) *T {
	return &v
}

func ToSlicePtrs[T any](in []T) []*T {
	out := make([]*T, len(in))
	for i := range in {
		out[i] = &in[i]
	}
	return out
}

func ToSliceValues[T any](in []*T) []T {
	out := make([]T, len(in))
	for i := range in {
		out[i] = *in[i]
	}
	return out
}
