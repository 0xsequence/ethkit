package ethwallet

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/go-ethereum/accounts"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
)

var DefaultWalletOptions = WalletOptions{
	DerivationPath:             "m/44'/60'/0'/0/0",
	RandomWalletEntropyBitSize: EntropyBitSize12WordMnemonic,
}

type Wallet struct {
	hdnode         *HDNode
	provider       *ethrpc.Provider
	walletProvider *WalletProvider
}

type WalletOptions struct {
	DerivationPath             string
	RandomWalletEntropyBitSize int
}

func NewWalletFromPrivateKey(key string) (*Wallet, error) {
	hdnode, err := NewHDNodeFromPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return &Wallet{hdnode: hdnode}, nil
}

func NewWalletFromHDNode(hdnode *HDNode, optPath ...accounts.DerivationPath) (*Wallet, error) {
	var err error
	derivationPath := DefaultBaseDerivationPath
	if len(optPath) > 0 {
		derivationPath = optPath[0]
	}

	err = hdnode.DerivePath(derivationPath)
	if err != nil {
		return nil, err
	}

	return &Wallet{hdnode: hdnode}, nil
}

func NewWalletFromRandomEntropy(options ...WalletOptions) (*Wallet, error) {
	opts := DefaultWalletOptions
	if len(options) > 0 {
		opts = options[0]
	}

	derivationPath, err := ParseDerivationPath(opts.DerivationPath)
	if err != nil {
		return nil, err
	}

	hdnode, err := NewHDNodeFromRandomEntropy(opts.RandomWalletEntropyBitSize, &derivationPath)
	if err != nil {
		return nil, err
	}

	wallet, err := NewWalletFromHDNode(hdnode, derivationPath)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

func NewWalletFromMnemonic(mnemonic string, optPath ...string) (*Wallet, error) {
	var err error
	derivationPath := DefaultBaseDerivationPath
	if len(optPath) > 0 {
		derivationPath, err = ParseDerivationPath(optPath[0])
		if err != nil {
			return nil, err
		}
	}

	hdnode, err := NewHDNodeFromMnemonic(mnemonic, &derivationPath)
	if err != nil {
		return nil, err
	}

	wallet, err := NewWalletFromHDNode(hdnode, derivationPath)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

func (w *Wallet) Clone() (*Wallet, error) {
	hdnode, err := w.hdnode.Clone()
	if err != nil {
		return nil, err
	}
	return &Wallet{
		hdnode: hdnode, provider: w.provider,
	}, nil
}

func (w *Wallet) Transactor(ctx context.Context) (*bind.TransactOpts, error) {
	var chainID *big.Int
	if w.provider != nil {
		var err error
		chainID, err = w.provider.ChainID(ctx)
		if err != nil {
			if w.provider.Config.ChaindID != 0 {
				chainID = big.NewInt(int64(w.provider.Config.ChaindID))
			} else {
				return nil, err
			}
		}
	}
	return w.TransactorForChainID(chainID)
}

func (w *Wallet) TransactorForChainID(chainID *big.Int) (*bind.TransactOpts, error) {
	if chainID == nil {
		// This is deprecated and will log a warning since it uses the original Homestead signer
		return bind.NewKeyedTransactor(w.hdnode.PrivateKey()), nil
	} else {
		return bind.NewKeyedTransactorWithChainID(w.hdnode.PrivateKey(), chainID)
	}
}

func (w *Wallet) GetProvider() *ethrpc.Provider {
	return w.provider
}

func (w *Wallet) SetProvider(provider *ethrpc.Provider) {
	w.provider = provider

	if w.walletProvider == nil {
		w.walletProvider = &WalletProvider{wallet: w}
	}
	w.walletProvider.provider = provider
}

func (w *Wallet) Provider() *WalletProvider {
	return w.walletProvider
}

func (w *Wallet) SelfDerivePath(path accounts.DerivationPath) (common.Address, error) {
	err := w.hdnode.DerivePath(path)
	if err != nil {
		return common.Address{}, err
	}
	return w.hdnode.Address(), nil
}

func (w *Wallet) DerivePath(path accounts.DerivationPath) (*Wallet, common.Address, error) {
	wallet, err := w.Clone()
	if err != nil {
		return nil, common.Address{}, err
	}
	address, err := wallet.SelfDerivePath(path)
	return wallet, address, err
}

func (w *Wallet) SelfDerivePathFromString(path string) (common.Address, error) {
	err := w.hdnode.DerivePathFromString(path)
	if err != nil {
		return common.Address{}, err
	}
	return w.hdnode.Address(), nil
}

func (w *Wallet) DerivePathFromString(path string) (*Wallet, common.Address, error) {
	wallet, err := w.Clone()
	if err != nil {
		return nil, common.Address{}, err
	}
	address, err := wallet.SelfDerivePathFromString(path)
	return wallet, address, err
}

func (w *Wallet) SelfDeriveAccountIndex(accountIndex uint32) (common.Address, error) {
	err := w.hdnode.DeriveAccountIndex(accountIndex)
	if err != nil {
		return common.Address{}, err
	}
	return w.hdnode.Address(), nil
}

func (w *Wallet) DeriveAccountIndex(accountIndex uint32) (*Wallet, common.Address, error) {
	wallet, err := w.Clone()
	if err != nil {
		return nil, common.Address{}, err
	}
	address, err := wallet.SelfDeriveAccountIndex(accountIndex)
	return wallet, address, err
}

func (w *Wallet) Address() common.Address {
	return w.hdnode.Address()
}

func (w *Wallet) HDNode() *HDNode {
	return w.hdnode
}

func (w *Wallet) PrivateKey() *ecdsa.PrivateKey {
	return w.hdnode.PrivateKey()
}

func (w *Wallet) PublicKey() *ecdsa.PublicKey {
	return w.hdnode.PublicKey()
}

func (w *Wallet) PrivateKeyHex() string {
	privateKeyBytes := crypto.FromECDSA(w.hdnode.PrivateKey())
	return hexutil.Encode(privateKeyBytes)
}

func (w *Wallet) PublicKeyHex() string {
	publicKeyBytes := crypto.FromECDSAPub(w.hdnode.PublicKey())
	return hexutil.Encode(publicKeyBytes)
}

func (w *Wallet) SignTx(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	signer := types.LatestSignerForChainID(chainID)
	signedTx, err := types.SignTx(tx, signer, w.hdnode.PrivateKey())
	if err != nil {
		return nil, err
	}

	msg, err := signedTx.AsMessage(signer, nil)
	if err != nil {
		return nil, err
	}

	sender := msg.From()
	if sender != w.hdnode.Address() {
		return nil, fmt.Errorf("signer mismatch: expected %s, got %s", w.hdnode.Address().Hex(), sender.Hex())
	}

	return signedTx, nil
}

func (w *Wallet) SignMessage(msg []byte) ([]byte, error) {
	m := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	h := crypto.Keccak256([]byte(m))

	sig, err := crypto.Sign(h, w.hdnode.PrivateKey())
	if err != nil {
		return []byte{}, err
	}
	sig[64] += 27

	return sig, nil
}

func (w *Wallet) IsValidSignature(msg, sig []byte) (bool, error) {
	recoveredAddress, err := RecoverAddress(msg, sig)
	if err != nil {
		return false, err
	}
	if recoveredAddress == w.Address() {
		return true, nil
	}
	return false, fmt.Errorf("signature does not match recovered address for this message")
}

func (w *Wallet) IsValidSignatureOfDigest(digest, sig []byte) (bool, error) {
	recoveredAddress, err := RecoverAddressFromDigest(digest, sig)
	if err != nil {
		return false, err
	}
	if recoveredAddress == w.Address() {
		return true, nil
	}
	return false, fmt.Errorf("signature does not match recovered address for this message digest")
}

func (w *Wallet) NewTransaction(ctx context.Context, txnRequest *ethtxn.TransactionRequest) (*types.Transaction, error) {
	if txnRequest == nil {
		return nil, fmt.Errorf("ethwallet: txnRequest is required")
	}

	provider := w.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("ethwallet: provider is not set")
	}

	chainID, err := provider.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("ethwallet: %w", err)
	}

	txnRequest.From = w.Address()

	rawTx, err := ethtxn.NewTransaction(ctx, provider, txnRequest)
	if err != nil {
		return nil, err
	}

	signedTx, err := w.SignTx(rawTx, chainID)
	if err != nil {
		return nil, fmt.Errorf("ethwallet: %w", err)
	}

	return signedTx, nil
}

func (w *Wallet) SendTransaction(ctx context.Context, signedTx *types.Transaction) (*types.Transaction, ethtxn.WaitReceipt, error) {
	provider := w.GetProvider()
	if provider == nil {
		return nil, nil, fmt.Errorf("ethwallet (SendTransaction): provider is not set")
	}
	return ethtxn.SendTransaction(ctx, provider, signedTx)
}
