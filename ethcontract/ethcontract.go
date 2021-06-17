package ethcontract

import (
	"fmt"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
	"github.com/0xsequence/ethkit/go-ethereum/common"
)

type Contract struct {
	*bind.BoundContract
	address common.Address
	abi     abi.ABI
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
		address:       address,
	}
	return contract
}

func (c *Contract) Address() common.Address {
	return c.address
}

func (c *Contract) ABI() abi.ABI {
	return c.abi
}

func (c *Contract) Encode(method string, args ...interface{}) ([]byte, error) {
	m, ok := c.abi.Methods[method]
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
