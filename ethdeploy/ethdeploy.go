package ethdeploy

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/horizon-games/ethkit/ethrpc"
	"github.com/horizon-games/ethkit/ethwallet"
)

// Deploy any contract with just the abi and its bytecode
//
// Sample usage:
//
// auth, err := wallet.Transactor()
// assert.NoError(t, err)
// auth.Nonce = big.NewInt(int64(nonce)) // next nonce for the account
// auth.Value = big.NewInt(0)            // in wei
// auth.GasLimit = uint64(5000000)       // in units
// auth.GasPrice = gasPrice
//
// // deploy transaction
// contractAddress, tx, contract, err := ethdeploy.DeployContract(auth, service.ETHRPC, contractABIJSONString, contractBytecodeHex)
//
// // get another account from the list
// wallet2, _ := ethwallet.NewETHWallet(cfg.Wallet.PrivateKeyMnemonic)
// wallet2.SetAccountIndex(1)
// address2, _ := wallet2.Address()
//
// var out = new(*big.Int)
// err := contract.Call(&bind.CallOpts{Context: context.Background()}, out, "balanceOf", address2, big.NewInt(1))
// assert.NoError(t, err)
// fmt.Println("=======>", *out) // <---<< returns `balanceOf(address2)` output from the chain
//

type Deployer struct {
	ETHRPC *ethrpc.JSONRPC
}

func NewDeployer(ethrpc *ethrpc.JSONRPC) *Deployer {
	return &Deployer{ETHRPC: ethrpc}
}

// TODO: accept optional *TransactOpts argument, can be nil and we'll populate ourselves
// or make our own structs like DeployOpts with nonce, gasPrice and gasLimit
func (d *Deployer) DeployContract(ctx context.Context, wallet *ethwallet.Wallet, contractABI, contractBytecodeHex string) (common.Address, *types.Transaction, *bind.BoundContract, error) {
	address, err := wallet.Address()
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	nonce, err := d.ETHRPC.PendingNonceAt(ctx, address)
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	gasPrice, err := d.ETHRPC.SuggestGasPrice(ctx)
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	auth, err := wallet.Transactor()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)
	auth.GasLimit = uint64(5000000)
	auth.GasPrice = gasPrice

	return DeployContract(auth, d.ETHRPC, contractABI, contractBytecodeHex)
}

func DeployContract(auth *bind.TransactOpts, backend bind.ContractBackend, contractABI, contractBytecodeHex string) (common.Address, *types.Transaction, *bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(contractABI))
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	address, tx, contract, err := bind.DeployContract(auth, parsed, common.FromHex(contractBytecodeHex), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	return address, tx, contract, nil
}
