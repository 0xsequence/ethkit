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

func StringifyValues(values []interface{}) ([]string, error) {
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
			break

		case string:
			strs = append(strs, v)
			break

		default:
			strs = append(strs, fmt.Sprintf("%v", value))
			break
		}
	}

	return strs, nil
}
