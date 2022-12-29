package ethcontract

import (
	"fmt"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
	"github.com/0xsequence/ethkit/go-ethereum/common"
)

type Contract struct {
	*bind.BoundContract
	Address common.Address
	ABI     abi.ABI
}

func NewContractCaller(address common.Address, abi abi.ABI, caller bind.ContractCaller) *Contract {
	return NewContract(address, abi, caller, nil, nil)
}

func NewContractTransactor(address common.Address, abi abi.ABI, caller bind.ContractCaller, transactor bind.ContractTransactor) *Contract {
	return NewContract(address, abi, caller, transactor, nil)
}

func NewContractFilterer(address common.Address, abi abi.ABI, filterer bind.ContractFilterer) *Contract {
	return NewContract(address, abi, nil, nil, filterer)
}

func NewContract(address common.Address, abi abi.ABI, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) *Contract {
	contract := &Contract{
		BoundContract: bind.NewBoundContract(address, abi, caller, transactor, filterer),
		Address:       address,
		ABI:           abi,
	}
	return contract
}

func (c *Contract) Encode(method string, args ...interface{}) ([]byte, error) {
	m, ok := c.ABI.Methods[method]
	if !ok {
		return nil, fmt.Errorf("contract method %s not found", method)
	}
	input, err := m.Inputs.Pack(args...)
	if err != nil {
		return nil, err
	}
	input = append(m.ID, input...)
	return input, nil
}

func (c *Contract) EventTopicHash(eventName string) (ethkit.Hash, error) {
	ev, ok := c.ABI.Events[eventName]
	if !ok {
		return ethkit.Hash{}, fmt.Errorf("ethcontract: event '%s' not found in contract abi", eventName)
	}
	h := ethcoder.Keccak256Hash([]byte(ev.Sig))
	return h, nil
}
