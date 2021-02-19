package ethcoder

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
)

func HexEncode(h []byte) string {
	return hexutil.Encode(h)
}

func HexDecode(h string) ([]byte, error) {
	return hexutil.Decode(h)
}

func MustHexDecode(h string) []byte {
	b, err := HexDecode(h)
	if err != nil {
		panic(fmt.Errorf("ethcoder: must hex decode but failed due to, %v", err))
	}
	return b
}

func HexDecodeBytes32(h string) ([32]byte, error) {
	slice, err := hexutil.Decode(h)
	if err != nil {
		return [32]byte{}, err
	}
	if len(slice) != 32 {
		return [32]byte{}, errors.New("hex input is not 32 bytes")
	}

	return BytesToBytes32(slice), nil
}

func HexDecodeBigIntArray(bigNumsHex []string) ([]*big.Int, error) {
	var err error
	nums := make([]*big.Int, len(bigNumsHex))
	for i := 0; i < len(bigNumsHex); i++ {
		nums[i], err = hexutil.DecodeBig(bigNumsHex[i])
		if err != nil {
			return nil, err
		}
	}
	return nums, nil
}

func HexEncodeBigIntArray(bigNums []*big.Int) ([]string, error) {
	nums := make([]string, len(bigNums))
	for i := 0; i < len(bigNums); i++ {
		nums[i] = hexutil.EncodeBig(bigNums[i])
	}
	return nums, nil
}

func HexTrimLeadingZeros(hex string) (string, error) {
	if hex[0:2] != "0x" {
		return "", errors.New("ethcoder: expecting hex value")
	}
	hex = fmt.Sprintf("0x%s", strings.TrimLeft(hex[2:], "0"))
	if hex == "0x" {
		return "0x0", nil
	} else {
		return hex, nil
	}
}
