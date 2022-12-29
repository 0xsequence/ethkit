package ethcoder

import (
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"golang.org/x/crypto/sha3"
)

func Keccak256Hash(input []byte) common.Hash {
	return common.BytesToHash(Keccak256(input))
}

func Keccak256(input []byte) []byte {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(input)
	return hasher.Sum(nil)
}

func SHA3(input []byte) common.Hash {
	return Keccak256Hash(input)
}
