package ethtest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/ethcontract"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Testchain struct {
	options TestchainOptions

	chainID        *big.Int         // chainID determined by the test chain
	walletMnemonic string           // test wallet mnemonic parsed from package.json
	Provider       *ethrpc.Provider // provider rpc to the test chain
}

type TestchainOptions struct {
	NodeURL string
}

var DefaultTestchainOptions = TestchainOptions{
	NodeURL: "http://localhost:8545",
}

func NewTestchain(opts ...TestchainOptions) (*Testchain, error) {
	var err error
	tc := &Testchain{}

	// set options
	if len(opts) > 0 {
		tc.options = opts[0]
	} else {
		tc.options = DefaultTestchainOptions
	}

	// provider
	tc.Provider, err = ethrpc.NewProvider(tc.options.NodeURL)
	if err != nil {
		return nil, err
	}

	// connect to the test-chain or error out if fail to communicate
	if err := tc.connect(); err != nil {
		return nil, err
	}

	return tc, nil
}

func (c *Testchain) connect() error {
	numAttempts := 6
	for i := 0; i < numAttempts; i++ {
		chainID, err := c.Provider.ChainID(context.Background())
		if err != nil || chainID == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		c.chainID = chainID
	}
	if c.chainID == nil {
		return fmt.Errorf("ethtest: unable to connect to testchain")
	}
	return nil
}

func (c *Testchain) ChainID() *big.Int {
	return c.chainID
}

func (c *Testchain) Wallet() (*ethwallet.Wallet, error) {
	var err error

	if c.walletMnemonic == "" {
		c.walletMnemonic, err = parseTestWalletMnemonic()
		if err != nil {
			return nil, err
		}
	}

	// we create a new instance each time so we don't change the account indexes
	// on the wallet across consumers
	wallet, err := ethwallet.NewWalletFromMnemonic(c.walletMnemonic)
	if err != nil {
		return nil, err
	}
	wallet.SetProvider(c.Provider)

	err = c.FundAddress(wallet.Address())
	if err != nil {
		return nil, err
	}

	return wallet, nil
}

func (c *Testchain) MustWallet(optAccountIndex ...uint32) *ethwallet.Wallet {
	wallet, err := c.Wallet()
	if err != nil {
		panic(err)
	}
	if len(optAccountIndex) > 0 {
		_, err = wallet.SelfDeriveAccountIndex(optAccountIndex[0])
		if err != nil {
			panic(err)
		}
	}

	err = c.FundAddress(wallet.Address())
	if err != nil {
		panic(err)
	}

	return wallet
}

func (c *Testchain) DummyWallet(seed uint64) (*ethwallet.Wallet, error) {
	wallet, err := ethwallet.NewWalletFromPrivateKey(DummyPrivateKey(seed))
	if err != nil {
		return nil, err
	}
	wallet.SetProvider(c.Provider)
	return wallet, nil
}

func (c *Testchain) DummyWallets(nWallets uint64, startingSeed uint64) ([]*ethwallet.Wallet, error) {
	var wallets []*ethwallet.Wallet

	for i := uint64(0); i < nWallets; i++ {
		wallet, err := c.DummyWallet(startingSeed + i*1000)
		if err != nil {
			return nil, err
		}
		wallets = append(wallets, wallet)
	}

	return wallets, nil
}

func (c *Testchain) FundWallets(minBalance float64, wallets ...*ethwallet.Wallet) error {
	minTarget := ETHValue(minBalance)
	fundAddresses := []ethkit.Address{}

	for _, wallet := range wallets {
		balance, err := c.Provider.BalanceAt(context.Background(), wallet.Address(), nil)
		if err != nil {
			return err
		}
		if balance.Cmp(minTarget) < 0 {
			fundAddresses = append(fundAddresses, wallet.Address())
		}
	}

	return c.FundAddresses(fundAddresses, minBalance)
}

func (c *Testchain) FundAddress(addr common.Address, optBalanceTarget ...float64) error {
	target := ETHValue(100)
	if len(optBalanceTarget) > 0 {
		target = ETHValue(optBalanceTarget[0])
	}

	balance, err := c.Provider.BalanceAt(context.Background(), addr, nil)
	if err != nil {
		return err
	}

	if balance.Cmp(target) >= 0 {
		// skip, we have enough funds in this wallet for the target
		return nil
	}

	var accounts []common.Address
	call := ethrpc.NewCallBuilder[[]common.Address]("eth_accounts", nil)
	_, err = c.Provider.Do(context.Background(), call.Into(&accounts))
	if err != nil {
		return err
	}

	type SendTx struct {
		From  *common.Address `json:"from"`
		To    *common.Address `json:"to"`
		Value string          `json:"value"`
	}

	amount := big.NewInt(0)
	amount.Sub(target, balance)
	// if balance.Cmp(target) < 0 {
	// 	// top up to the target
	// 	amount.Sub(target, balance)
	// } else {
	// 	// already at the target, add same target quantity
	// 	amount.Set(target)
	// }

	tx := &SendTx{
		From:  &accounts[0],
		To:    &addr,
		Value: "0x" + amount.Text(16),
	}

	_, err = c.Provider.Do(context.Background(), ethrpc.NewCall("eth_sendTransaction", tx))
	if err != nil {
		return err
	}

	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		balance, err = c.Provider.BalanceAt(context.Background(), addr, nil)
		if err != nil {
			return err
		}
		if balance.Cmp(target) >= 0 {
			return nil
		}
	}

	return fmt.Errorf("test wallet failed to fund")
}

func (c *Testchain) MustFundAddress(addr common.Address, optBalanceTarget ...float64) {
	err := c.FundAddress(addr, optBalanceTarget...)
	if err != nil {
		panic(err)
	}
}

func (c *Testchain) FundAddresses(addrs []common.Address, optBalanceTarget ...float64) error {
	for _, addr := range addrs {
		err := c.FundAddress(addr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Testchain) GetDeployWallet() *ethwallet.Wallet {
	return c.MustWallet(5)
}

// GetDeployTransactor returns a account transactor typically used for deploying contracts
func (c *Testchain) GetDeployTransactor() (*bind.TransactOpts, error) {
	return c.GetDeployWallet().Transactor(context.Background())
}

// GetRelayerWallet is the wallet dedicated EOA wallet to relaying transactions
func (c *Testchain) GetRelayerWallet() *ethwallet.Wallet {
	return c.MustWallet(6)
}

// Deploy will deploy a contract registered in `Contracts` registry using the standard deployment method. Each Deploy call
// will instanitate a new contract on the test chain.
func (c *Testchain) Deploy(t *testing.T, contractName string, contractConstructorArgs ...interface{}) (*ethcontract.Contract, *types.Receipt) {
	artifact, ok := Contracts.Get(contractName)
	if !ok {
		t.Fatal(fmt.Errorf("contract abi not found for name %s", contractName))
	}

	data := make([]byte, len(artifact.Bin))
	copy(data, artifact.Bin)

	var input []byte
	var err error

	// encode constructor call
	if len(contractConstructorArgs) > 0 && len(artifact.ABI.Constructor.Inputs) > 0 {
		input, err = artifact.ABI.Pack("", contractConstructorArgs...)
	} else {
		input, err = artifact.ABI.Pack("")
	}
	if err != nil {
		t.Fatal(fmt.Errorf("contract constructor pack failed: %w", err))
	}

	// append constructor calldata at end of the contract bin
	data = append(data, input...)

	wallet := c.GetDeployWallet()
	signedTxn, err := wallet.NewTransaction(context.Background(), &ethtxn.TransactionRequest{
		Data: data,

		// NOTE: below are needed for geth. hardhat and anvil don't need them.
		// Recommendation is to use anvil for testing, or hardhat.
		GasLimit: 1_000_000,
		GasPrice: big.NewInt(1e9), // 1 gwei
	})
	if err != nil {
		t.Fatal(err)
	}
	_, waitTx, err := wallet.SendTransaction(context.Background(), signedTxn)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := waitTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatal(fmt.Errorf("txn failed: %w", err))
	}

	return ethcontract.NewContractCaller(receipt.ContractAddress, artifact.ABI, c.Provider), receipt
}

func (c *Testchain) WaitMined(txn common.Hash) error {
	_, err := ethrpc.WaitForTxnReceipt(context.Background(), c.Provider, txn)
	return err
}

func (c *Testchain) RandomNonce() *big.Int {
	space := big.NewInt(int64(time.Now().Nanosecond()))
	return space
}

// parseTestWalletMnemonic parses the wallet mnemonic from ./package.json, the same
// key used to start the test chain server.
func parseTestWalletMnemonic() (string, error) {
	_, filename, _, _ := runtime.Caller(0)
	cwd := filepath.Dir(filename)

	packageJSONFile := filepath.Join(cwd, "./testchain/package.json")
	data, err := os.ReadFile(packageJSONFile)
	if err != nil {
		return "", fmt.Errorf("ParseTestWalletMnemonic, read: %w", err)
	}

	var dict struct {
		Config struct {
			Mnemonic string `json:"mnemonic"`
		} `json:"config"`
	}
	err = json.Unmarshal(data, &dict)
	if err != nil {
		return "", fmt.Errorf("ParseTestWalletMnemonic, unmarshal: %w", err)
	}

	return dict.Config.Mnemonic, nil
}
