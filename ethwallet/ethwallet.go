package ethwallet

import "github.com/ethereum/go-ethereum/accounts"

type Wallet struct {
	hdnode *HDNode
	// jsonrpc *ethrpc.JSONRPC
}

func NewWalletFromHDNode(hdnode *HDNode, path string) (*Wallet, error) {
	var err error
	var derivationPath accounts.DerivationPath

	if path == "" {
		derivationPath = DefaultBaseDerivationPath
	} else {
		derivationPath, err = ParseDerivationPath(path)
	}

	err = hdnode.DerivePath(derivationPath)
	if err != nil {
		return nil, err
	}

	return &Wallet{hdnode: hdnode}, nil
}

func NewWalletFromRandomEntropy(bitSize int, path string) (*Wallet, error) {
	hdnode, err := NewHDNodeFromRandomEntropy(bitSize, nil)
	if err != nil {
		return nil, err
	}
	return NewWalletFromHDNode(hdnode, path)
}

func NewWalletFromMnemonic(mnemonic string, path string) (*Wallet, error) {
	hdnode, err := NewHDNodeFromMnemonic(mnemonic, nil)
	if err != nil {
		return nil, err
	}
	return NewWalletFromHDNode(hdnode, path)
}

//

// Signer(), aka Transactor() ?

// Provider()

// GetAddress()

// Sign(tx)

// SignMessage(string)

// GetBalance()

// GetTransactionCount()

// ..

// func (w *Wallet) URL() accounts.URL {
// 	return accounts.URL{}
// }

// func (w *Wallet) Status() (string, error) {
// 	return "", nil
// }

// func (w *Wallet) Open(passphrase string) error {
// 	return nil
// }

// func (w *Wallet) Close() error {
// 	return nil
// }

// func (w *Wallet) Accounts() []accounts.Account {
// 	return nil
// }

// func (w *Wallet) Contains(account accounts.Account) bool {
// 	return false
// }

// func (w *Wallet) Derive(path accounts.DerivationPath, pin bool) (accounts.Account, error) {
// 	return accounts.Account{}, nil
// }

// func (w *Wallet) SelfDerive(bases []accounts.DerivationPath, chain ethereum.ChainStateReader) {

// }

// func (w *Wallet) SignData(account accounts.Account, mimeType string, data []byte) ([]byte, error) {
// 	return nil, nil
// }

// func (w *Wallet) SignDataWithPassphrase(account accounts.Account, passphrase, mimeType string, data []byte) ([]byte, error) {
// 	return nil, nil
// }

// func (w *Wallet) SignText(account accounts.Account, text []byte) ([]byte, error) {
// 	return nil, nil
// }

// func (w *Wallet) SignTextWithPassphrase(account accounts.Account, passphrase string, hash []byte) ([]byte, error) {
// 	return nil, nil
// }

// func (w *Wallet) SignTx(account accounts.Account, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {

// }

// func (w *Wallet) SignTxWithPassphrase(account accounts.Account, passphrase string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
// 	return nil, nil
// }
