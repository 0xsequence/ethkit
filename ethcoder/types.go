package ethcoder

import "github.com/0xsequence/ethkit/go-ethereum/accounts/abi"

func MustNewType(str string) abi.Type {
	typ, err := abi.NewType(str, "", nil)
	if err != nil {
		panic(err)
	}
	return typ
}

func MustNewArrayTypeTuple(components []abi.ArgumentMarshaling) abi.Type {
	typ, err := abi.NewType("tuple[]", "", components)
	if err != nil {
		panic(err)
	}
	return typ
}
