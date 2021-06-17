package ethcontract

import (
	"fmt"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
)

func ParseABI(abiJSON string) (abi.ABI, error) {
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return abi.ABI{}, fmt.Errorf("unable to parse abi json: %w", err)
	}
	return parsed, nil
}

func MustParseABI(abiJSON string) abi.ABI {
	parsed, err := ParseABI(abiJSON)
	if err != nil {
		panic(err)
	}
	return parsed
}
