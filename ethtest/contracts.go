package ethtest

import (
	"context"
	_ "embed"

	"github.com/0xsequence/ethkit/ethartifact"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

var (
	//go:embed contracts/CallReceiverMock.json
	artifact_callReceiverMock string

	//go:embed contracts/ERC20Mock.json
	artifact_erc20mock string

	// Contracts registry to have some contracts on hand during testing
	Contracts = ethartifact.NewContractRegistry()
)

func init() {
	Contracts.MustAdd(ethartifact.MustParseArtifactJSON(artifact_callReceiverMock))
	Contracts.MustAdd(ethartifact.MustParseArtifactJSON(artifact_erc20mock))
}

func ContractCall(provider *ethrpc.Provider, contractAddress common.Address, contractABI abi.ABI, result interface{}, method string, args ...interface{}) ([]byte, error) {
	calldata, err := contractABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{
		To:   &contractAddress,
		Data: calldata,
	}

	output, err := provider.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return output, nil
	}

	err = contractABI.UnpackIntoInterface(result, method, output)
	if err != nil {
		return output, err
	}
	return output, nil
}

func ContractQuery(provider *ethrpc.Provider, contractAddress common.Address, inputExpr, outputExpr string, args []string) ([]string, error) {
	return provider.QueryContract(context.Background(), contractAddress.Hex(), inputExpr, outputExpr, args)
}

func ContractTransact(signer *ethwallet.Wallet, contractAddress common.Address, contractABI abi.ABI, method string, args ...interface{}) (*types.Receipt, error) {
	calldata, err := contractABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}

	signedTxn, err := signer.NewTransaction(context.Background(), &ethtxn.TransactionRequest{
		To:   &contractAddress,
		Data: calldata,
	})
	if err != nil {
		return nil, err
	}

	_, waitReceipt, err := signer.SendTransaction(context.Background(), signedTxn)
	if err != nil {
		return nil, err
	}

	receipt, err := waitReceipt(context.Background())
	if err != nil {
		return nil, err
	}

	return receipt, nil
}
