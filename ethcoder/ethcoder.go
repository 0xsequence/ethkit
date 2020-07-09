package ethcoder

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func Keccak256(input []byte) []byte {
	return crypto.Keccak256(input)
}

func Sha3HashFromBytes(input []byte) string {
	buf := crypto.Keccak256(hexutil.Bytes(input))
	return fmt.Sprintf("0x%x", buf)
}

func Sha3Hash(input string) string {
	buf := crypto.Keccak256(hexutil.Bytes([]byte(input)))
	return fmt.Sprintf("0x%x", buf)
}

func BytesToBytes32(slice []byte) [32]byte {
	var bytes32 [32]byte
	copy(bytes32[:], slice)
	return bytes32
}

func AddressPadding(input string) string {
	if strings.HasPrefix(input, "0x") {
		input = input[2:]
	}
	if len(input) < 64 {
		input = strings.Repeat("0", 64-len(input)) + input
	}
	return input[0:64]
}

func FunctionSignature(functionExpr string) string {
	return HexEncode(Keccak256([]byte(functionExpr))[0:4])
}
