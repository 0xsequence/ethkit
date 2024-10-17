package ethcoder

import (
	"fmt"
	"strings"
)

func BytesToBytes32(slice []byte) [32]byte {
	var bytes32 [32]byte
	copy(bytes32[:], slice)
	return bytes32
}

func PaddedAddress(address string) string {
	if strings.HasPrefix(address, "0x") {
		address = address[2:]
	}
	if len(address) < 64 {
		address = strings.Repeat("0", 64-len(address)) + address
	}
	return address[0:64]
}

func FunctionSignature(functionExpr string) string {
	return HexEncode(Keccak256([]byte(functionExpr))[0:4])
}

func StringifyValues(values []any) ([]string, error) {
	strs := []string{}

	for _, value := range values {
		stringer, ok := value.(fmt.Stringer)
		if ok {
			strs = append(strs, stringer.String())
			continue
		}

		switch v := value.(type) {
		case nil:
			strs = append(strs, "")
		case string:
			strs = append(strs, v)
		default:
			strs = append(strs, fmt.Sprintf("%v", value))
		}
	}

	return strs, nil
}
