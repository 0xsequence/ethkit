package ethcoder

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func AbiCoder(argTypes []string, argValues []interface{}) ([]byte, error) {
	if len(argTypes) != len(argValues) {
		return nil, errors.New("invalid arguments - types and values do not match")
	}

	args := abi.Arguments{}
	for _, argType := range argTypes {
		abiType, err := abi.NewType(argType, nil)
		if err != nil {
			return nil, err
		}
		args = append(args, abi.Argument{Type: abiType})
	}
	return args.Pack(argValues...)
}

func AbiCoderHex(argTypes []string, argValues []interface{}) (string, error) {
	b, err := AbiCoder(argTypes, argValues)
	if err != nil {
		return "", err
	}
	h := hexutil.Encode(b)
	return h, nil
}

func SolidityPack(argTypes []string, argValues []interface{}) ([]byte, error) {
	if len(argTypes) != len(argValues) {
		return nil, errors.New("invalid arguments - types and values do not match")
	}

	pack := []byte{}
	for i := 0; i < len(argTypes); i++ {
		b, err := solidityArgumentPack(argTypes[i], argValues[i], false)
		if err != nil {
			return nil, err
		}
		pack = append(pack, b...)
	}
	return pack, nil
}

func SolidityPackHex(argTypes []string, argValues []interface{}) (string, error) {
	b, err := SolidityPack(argTypes, argValues)
	if err != nil {
		return "", err
	}
	h := hexutil.Encode(b)
	return h, nil
}

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

func HexDecode(h string) ([]byte, error) {
	return hexutil.Decode(h)
}

func HexEncode(h []byte) string {
	return hexutil.Encode(h)
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
