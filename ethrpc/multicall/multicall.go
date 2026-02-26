//go:generate go run ../../cmd/ethkit abigen --abiFile IMulticall3.json --pkg multicall --type IMulticall3 --outFile imulticall3.gen.go

package multicall

import (
	"fmt"
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
)

type Call struct {
	Multicall3Call3Value

	Outputs []Output
}

type Output struct {
	Type abi.Argument
	// Output may be nil to ignore the decoded value. When set, it must be a
	// non-nil pointer to the exact Go type returned by ABI decoding.
	Output any
}

func UnpackOutputs(data []byte, outputs ...Output) error {
	if len(outputs) == 0 {
		return nil
	}

	needsAssign := false
	for _, output := range outputs {
		if output.Output != nil {
			needsAssign = true
			break
		}
	}
	if !needsAssign {
		return nil
	}

	args := make(abi.Arguments, len(outputs))
	for i, output := range outputs {
		args[i] = output.Type
	}

	values, err := args.Unpack(data)
	if err != nil {
		return err
	}
	if len(values) != len(outputs) {
		return fmt.Errorf("expected %d outputs, got %d", len(outputs), len(values))
	}

	for i, output := range outputs {
		if output.Output == nil {
			continue
		}
		args := abi.Arguments{output.Type}
		if err := args.Copy(output.Output, []any{values[i]}); err != nil {
			return fmt.Errorf("output %d: %w", i, err)
		}
	}

	return nil
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
