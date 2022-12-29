package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/keystore"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

func init() {
	wallet := &wallet{}
	cmd := &cobra.Command{
		Use:   "wallet",
		Short: "EOA wallet",
		Run:   wallet.Run,
	}

	cmd.Flags().String("keyfile", "", "wallet key file path")
	cmd.Flags().Bool("new", false, "create a new wallet and save it to the keyfile")
	cmd.Flags().Bool("print-account", true, "print wallet account address from keyfile")
	cmd.Flags().Bool("print-mnemonic", false, "print wallet secret mnemonic from keyfile (danger!)")
	cmd.Flags().Bool("print-private-key", false, "print wallet private key from keyfile (danger!)")
	cmd.Flags().Bool("import-mnemonic", false, "import a secret mnemonic to a new keyfile")
	cmd.Flags().String("path", "", fmt.Sprintf("set derivation path, default: %s", ethwallet.DefaultWalletOptions.DerivationPath))

	rootCmd.AddCommand(cmd)
}

type wallet struct {
	// flags
	fKeyFile         string
	fCreateNew       bool
	fPrintAccount    bool
	fPrintMnemonic   bool
	fPrintPrivateKey bool
	fImportMnemonic  bool
	fPath            string

	// wallet key file
	keyFile walletKeyFile
	wallet  *ethwallet.Wallet
}

func (c *wallet) Run(cmd *cobra.Command, args []string) {
	c.fKeyFile, _ = cmd.Flags().GetString("keyfile")
	c.fCreateNew, _ = cmd.Flags().GetBool("new")
	c.fPrintAccount, _ = cmd.Flags().GetBool("print-account")
	c.fPrintMnemonic, _ = cmd.Flags().GetBool("print-mnemonic")
	c.fPrintPrivateKey, _ = cmd.Flags().GetBool("print-private-key")
	c.fImportMnemonic, _ = cmd.Flags().GetBool("import-mnemonic")
	c.fPath, _ = cmd.Flags().GetString("path")

	if c.fKeyFile == "" {
		fmt.Println("error: please pass --keyfile")
		help(cmd)
		return
	}
	if fileExists(c.fKeyFile) && (c.fCreateNew || c.fImportMnemonic) {
		fmt.Println("error: keyfile already exists on this filename, for safety we do not overwrite existing keyfiles.")
		help(cmd)
		return
	}
	if !c.fCreateNew && !c.fImportMnemonic && !c.fPrintMnemonic && !c.fPrintPrivateKey && !c.fPrintAccount {
		fmt.Println("error: not enough options provided to ethkit cli.")
		help(cmd)
		return
	}

	// Gen new wallet
	if c.fCreateNew || c.fImportMnemonic {
		if err := c.createNew(); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Load wallet from the key file
	data, err := os.ReadFile(c.fKeyFile)
	if err != nil {
		log.Fatal(err)
	}
	keyFile := walletKeyFile{}
	err = json.Unmarshal(data, &keyFile)
	if err != nil {
		log.Fatal(err)
	}
	c.keyFile = keyFile

	derivationPath := c.fPath
	if derivationPath == "" {
		derivationPath = c.keyFile.Path
	}

	pw, err := readSecretInput("Password: ")
	if err != nil {
		log.Fatal(err)
	}

	cipherText, err := keystore.DecryptDataV3(c.keyFile.Crypto, string(pw))
	if err != nil {
		log.Fatal(err)
	}

	wallet, err := ethwallet.NewWalletFromMnemonic(string(cipherText), derivationPath)
	if err != nil {
		log.Fatal(err)
	}
	c.wallet = wallet

	// Print mnemonic
	if c.fPrintMnemonic {
		if err := c.printMnemonic(); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Print private key
	if c.fPrintPrivateKey {
		if err := c.printPrivateKey(); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Print acconut address
	if c.fPrintAccount {
		if err := c.printAccount(); err != nil {
			log.Fatal(err)
		}
		return
	}
}

func (c *wallet) printMnemonic() error {
	fmt.Println("")
	fmt.Println("=> Your Ethereum private mnemonic is:")
	fmt.Println("=>", c.wallet.HDNode().Mnemonic())
	fmt.Println("")
	return nil
}

func (c *wallet) printPrivateKey() error {
	fmt.Println("")
	fmt.Println("=> Your Ethereum private key is:")
	fmt.Println("=>", c.wallet.PrivateKeyHex())
	fmt.Println("")
	return nil
}

func (c *wallet) printAccount() error {
	fmt.Println("")
	fmt.Println("")
	fmt.Println("=> Your Ethereum wallet address is:", c.wallet.Address().String())
	fmt.Println("")
	return nil
}

func (c *wallet) createNew() error {
	var err error
	var importMnemonic string

	if c.fImportMnemonic {
		var mnemonic []byte
		// TODO: use crypto/terminal and print *'s on each keypress of input
		mnemonic, err = readPlainInput("Enter your mnemonic to import: ")
		if err != nil {
			return err
		}
		importMnemonic = strings.TrimSpace(string(mnemonic))
	}

	derivationPath := c.fPath
	if derivationPath == "" {
		derivationPath = ethwallet.DefaultWalletOptions.DerivationPath
	}

	c.wallet, err = getWallet(importMnemonic, derivationPath)
	if err != nil {
		return err
	}

	pw, err := readSecretInput("Password: ")
	if err != nil {
		return err
	}
	if len(pw) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	fmt.Println("")
	confirmPw, err := readSecretInput("Confirm Password: ")
	if err != nil {
		return err
	}
	if string(pw) != string(confirmPw) {
		return errors.New("passwords do not match")
	}

	cryptoJSON, err := keystore.EncryptDataV3([]byte(c.wallet.HDNode().Mnemonic()), pw, keystore.StandardScryptN, keystore.StandardScryptP)
	if err != nil {
		return err
	}

	keyFile := walletKeyFile{
		Address: c.wallet.Address(),
		Path:    c.wallet.HDNode().DerivationPath().String(),
		Crypto:  cryptoJSON,
		Client:  fmt.Sprintf("ethkit/%s - github.com/0xsequence/ethkit", VERSION),
	}

	data, err := json.MarshalIndent(keyFile, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, []byte("\n")...)

	if err := os.WriteFile(c.fKeyFile, data, 0600); err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("")
	fmt.Println("=> success! ethkit has generated a new Ethereum wallet for you and saved")
	fmt.Println("=> it in an encrypted+password protected file at:")
	fmt.Println("=> ---")
	fmt.Println("=>", c.fKeyFile)
	fmt.Println("")
	fmt.Printf("=> to confirm, please run: ./ethkit wallet --keyfile=%s --print-account\n", c.fKeyFile)
	fmt.Println("")
	fmt.Println("=> Your new Ethereum wallet address is:", c.wallet.Address().String())
	fmt.Println("")

	return nil
}

type walletKeyFile struct {
	Address common.Address      `json:"address"`
	Path    string              `json:"path"`
	Crypto  keystore.CryptoJSON `json:"crypto"`
	Client  string              `json:"client"`
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func readSecretInput(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	password, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return nil, err
	}
	return password, nil
}

func readPlainInput(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return []byte(text), nil
}

func getWallet(mnemonic, derivationPath string) (*ethwallet.Wallet, error) {
	var err error
	var wallet *ethwallet.Wallet

	if derivationPath == "" {
		return nil, fmt.Errorf("derivationPath cannot be empty")
	}

	if mnemonic != "" {
		wallet, err = ethwallet.NewWalletFromMnemonic(mnemonic, derivationPath)
	} else {
		wallet, err = ethwallet.NewWalletFromRandomEntropy(ethwallet.WalletOptions{
			DerivationPath:             derivationPath,
			RandomWalletEntropyBitSize: ethwallet.EntropyBitSize24WordMnemonic,
		})
	}
	if err != nil {
		return nil, err
	}

	return wallet, nil
}
