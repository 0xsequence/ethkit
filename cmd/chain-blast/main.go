package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/util"
)

// https://explorer-mainnet.maticvigil.com/address/0xb59ba5A13f0fb106EA6094a1F69786AA69be1424/transactions

var (
	ETH_NODE_URL = "http://localhost:8545"

	// keep this private, but requires a wallet with ETH or MATIC (depending on network)
	PRIVATE_WALLET_MNEMONIC = ""

	// token contract for testing
	ERC20_TEST_CONTRACT = "0xCCCD8b34e94F52eDFAdA6e6Ae4AE1C1ab43F9D67"
)

func init() {
	testConfig, err := util.ReadTestConfig("../../ethkit-test.json")
	if err != nil {
		panic(err)
	}

	if testConfig["POLYGON_MAINNET_URL"] != "" {
		ETH_NODE_URL = testConfig["POLYGON_MAINNET_URL"]
	}

	PRIVATE_WALLET_MNEMONIC = testConfig["PRIVATE_WALLET_MNEMONIC"]
}

func main() {
	fmt.Println("chain-blast start")
	fmt.Println("")

	// Provider
	provider, err := ethrpc.NewProvider(ETH_NODE_URL)
	if err != nil {
		fatal(err, "provider setup")
	}

	// Wallet
	wallet, err := ethwallet.NewWalletFromMnemonic(PRIVATE_WALLET_MNEMONIC)
	// wallet, err := ethwallet.NewWalletFromRandomEntropy()
	if err != nil {
		fatal(err, "wallet setup")
	}
	wallet.SetProvider(provider)

	// Check wallet balance
	balance, err := provider.BalanceAt(context.Background(), wallet.Address(), nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=> wallet address", wallet.Address().String())
	fmt.Println("=> wallet balance", balance.Int64())

	if balance.Cmp(big.NewInt(0)) == 0 {
		fatal(nil, "wallet balance is 0")
	}

	// deploy new contract
	if ERC20_TEST_CONTRACT == "" {
		fmt.Println("")
		fmt.Println("ERC20_TEST_CONTRACT var is empty, need to deploy a new contract..")
		promptAreYouSure()

		contractAddress, err := deployERC20(wallet)
		if err != nil {
			fatal(err, "deployERC20 failed")
		}

		fmt.Println("Set ERC20_TEST_CONTRACT and rerun:", contractAddress.Hex())

		ERC20_TEST_CONTRACT = contractAddress.String()
	}

	// confirm contract is deployed
	code, err := provider.CodeAt(context.Background(), common.HexToAddress(ERC20_TEST_CONTRACT), nil)
	if err != nil {
		fatal(err, "codeAt %s contract failed", ERC20_TEST_CONTRACT)
	}
	if len(code) == 0 {
		fatal(nil, "codeAt %s contract failed", ERC20_TEST_CONTRACT)
	}
	fmt.Println("=> erc20 test contract: deployed")

	// run transfer test
	err = transferERC20s(wallet)
	if err != nil {
		fatal(err, "transferERC20s")
	}
}

func deployERC20(wallet *ethwallet.Wallet) (common.Address, error) {
	provider := wallet.GetProvider()
	auth, err := wallet.Transactor(context.Background())
	if err != nil {
		return common.Address{}, err
	}

	address, contractTxn, erc20, err := DeployERC20Mock(auth, provider)
	if err != nil {
		return common.Address{}, err
	}
	fmt.Println("Contract creation txn hash:", contractTxn.Hash().Hex())
	err = waitForTxn(provider, contractTxn.Hash())
	if err != nil {
		return common.Address{}, err
	}

	txn, err := erc20.MockMint(auth, wallet.Address(), big.NewInt(1000000000000))
	fmt.Println("erc20 mint txn hash:", txn.Hash().Hex())
	err = waitForTxn(provider, txn.Hash())
	if err != nil {
		return common.Address{}, err
	}

	return address, nil
}

var randomRecipient = "0x1234567890123456789012345678901234567890"

func transferERC20s(wallet *ethwallet.Wallet) error {
	provider := wallet.GetProvider()

	fmt.Println("")

	erc20, err := NewERC20Mock(common.HexToAddress(ERC20_TEST_CONTRACT), provider)
	if err != nil {
		fatal(err, "NewERC20Mock")
	}

	balance, err := erc20.BalanceOf(nil, wallet.Address())
	if err != nil {
		fatal(err, "balanceOf")
	}
	fmt.Println("=> wallet token balance", balance.Int64())

	if balance.Cmp(big.NewInt(0)) == 0 {
		fatal(nil, "wallet token balance is 0")
	}

	// get ready to blast
	txnCount, err := provider.NonceAt(context.Background(), wallet.Address(), nil)
	if err != nil {
		return err
	}
	nonce, err := provider.PendingNonceAt(context.Background(), wallet.Address())
	if err != nil {
		return err
	}
	fmt.Println("=> wallet txn count:", txnCount)
	fmt.Println("=> wallet latest nonce:", nonce)

	fmt.Println("")

	// blastNum := 5 // num txns at a time to dispatch
	numTxns := 10 // will send this many parallel txns

	// marks that we send a txn at a time, and wait for it..
	var waitForEachTxn bool
	// waitForEachTxn = true
	waitForEachTxn = false

	auth, err := wallet.Transactor(context.Background())
	if err != nil {
		return err
	}

	// TODO: lets use ethmempool + subscribeWithFilter, and listen for transactions as they come in

	for i := 0; i < numTxns; i++ {
		// increment nonce ourselves to send parallel txns
		auth.Nonce = big.NewInt(0).SetUint64(nonce)

		// dispatch the txn
		txn, err := erc20.Transfer(auth, common.HexToAddress(randomRecipient), big.NewInt(8))
		if err != nil {
			fatal(err, "transfer #%d failed", i)
		}
		fmt.Printf("Sent txn %d with hash %s\n", i, txn.Hash().Hex())

		if waitForEachTxn {
			startTime := time.Now()
			err = waitForTxn(provider, txn.Hash())
			if err != nil {
				fatal(err, "transfer wait failed for txn %s", txn.Hash().Hex())
			}
			fmt.Printf("Txn mined in %s\n", time.Now().Sub(startTime))
			fmt.Println("")
		}

		// increment nonce for next txn
		nonce += 1
	}

	// wallet balance is now..:
	balance, err = erc20.BalanceOf(nil, wallet.Address())
	if err != nil {
		fatal(err, "balanceOf")
	}
	fmt.Println("=> wallet token balance", balance.Int64())

	return nil
}

func promptAreYouSure() {
	fmt.Println("")
	fmt.Printf("Are you sure you'd like to deploy a new ERC20 contract? [y/n]: ")

	resp := ""
	fmt.Scanln(&resp)

	fmt.Println("")

	if resp != "y" {
		fmt.Println("okay, exiting..")
		os.Exit(0)
	}
}

func waitForTxn(provider *ethrpc.Provider, hash common.Hash) error {
	for {
		receipt, err := provider.TransactionReceipt(context.Background(), hash)
		if err == ethereum.NotFound {
			time.Sleep(1 * time.Second)
			continue
		}
		if err != nil {
			return err
		}

		if receipt.Status == types.ReceiptStatusSuccessful {
			return nil
		} else {
			fmt.Printf("txnHash %s failed", hash.Hex())
			return errors.New("txn failed")
		}
	}
}

func fatal(err error, msg string, a ...interface{}) {
	if err != nil {
		fmt.Println(fmt.Sprintf("fatal error! %s: %v", fmt.Sprintf(msg, a...), err))
	} else {
		fmt.Println(fmt.Sprintf("fatal error! %s", fmt.Sprintf(msg, a...)))
	}
	os.Exit(1)
}
