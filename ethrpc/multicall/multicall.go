//go:generate go run ../../cmd/ethkit abigen --abiFile IMulticall3.json --pkg multicall --type IMulticall3 --outFile imulticall3.gen.go

package multicall

import (
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
)

type Call struct {
	Multicall3Call3Value

	ReturnTypes abi.Arguments
	Outputs     any
}

func BaseFee() []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getBasefee")
	return data
}

func BlockHash(number *big.Int) []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getBlockHash", number)
	return data
}

func BlockNumber() []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getBlockNumber")
	return data
}

func ChainID() []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getChainId")
	return data
}

func BlockCoinbase() []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getCurrentBlockCoinbase")
	return data
}

func BlockDifficulty() []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getCurrentBlockDifficulty")
	return data
}

func BlockGasLimit() []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getCurrentBlockGasLimit")
	return data
}

func BlockTimestamp() []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getCurrentBlockTimestamp")
	return data
}

func Balance(address common.Address) []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getEthBalance", address)
	return data
}

func LastBlockHash() []byte {
	abi, _ := IMulticall3MetaData.GetAbi()
	data, _ := abi.Pack("getLastBlockHash")
	return data
}
