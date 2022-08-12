package ethwallet

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"

	"github.com/0xsequence/ethkit/go-ethereum/accounts"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/tyler-smith/go-bip39"
)

// DefaultBaseDerivationPath is the base path from which custom derivation endpoints
// are incremented. As such, the first account will be at m/44'/60'/0'/0/0, the second
// at m/44'/60'/0'/0/1, etc.
var DefaultBaseDerivationPath = accounts.DefaultBaseDerivationPath

// Entropy bit size constants for 12 and 24 word mnemonics
const (
	EntropyBitSize12WordMnemonic = 128
	EntropyBitSize24WordMnemonic = 256
)

type HDNode struct {
	masterKey  *hdkeychain.ExtendedKey
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey

	entropy        []byte
	mnemonic       string
	derivationPath accounts.DerivationPath

	address common.Address
}

func NewHDNodeFromPrivateKey(privateKey string) (*HDNode, error) {
	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, err
	}

	return &HDNode{
		privateKey: key,
		publicKey:  &key.PublicKey,
		address:    crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewHDNodeFromMnemonic(mnemonic string, path *accounts.DerivationPath) (*HDNode, error) {
	entropy, err := MnemonicToEntropy(mnemonic)
	if err != nil {
		return nil, err
	}

	hdnode, err := NewHDNodeFromEntropy(entropy, path)
	if err != nil {
		return nil, err
	}
	hdnode.mnemonic = mnemonic
	return hdnode, nil
}

func NewHDNodeFromEntropy(entropy []byte, path *accounts.DerivationPath) (*HDNode, error) {
	mnemonic, err := EntropyToMnemonic(entropy)
	if err != nil {
		return nil, err
	}

	seed, err := NewSeedFromMnemonic(mnemonic)
	if err != nil {
		return nil, err
	}

	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}

	var derivationPath accounts.DerivationPath
	if path == nil {
		derivationPath = DefaultBaseDerivationPath
	} else {
		derivationPath = *path
	}

	privateKey, err := derivePrivateKey(masterKey, derivationPath)
	if err != nil {
		return nil, err
	}
	publicKey, err := derivePublicKey(masterKey, derivationPath)
	if err != nil {
		return nil, err
	}
	address, err := deriveAddress(masterKey, derivationPath)
	if err != nil {
		return nil, err
	}

	return &HDNode{
		masterKey:      masterKey,
		privateKey:     privateKey,
		publicKey:      publicKey,
		entropy:        entropy,
		mnemonic:       mnemonic,
		derivationPath: derivationPath,
		address:        address,
	}, nil
}

func NewHDNodeFromRandomEntropy(bitSize int, path *accounts.DerivationPath) (*HDNode, error) {
	entropy, err := RandomEntropy(bitSize)
	if err != nil {
		return nil, err
	}
	return NewHDNodeFromEntropy(entropy, path)
}

// NewSeedFromMnemonic returns a BIP-39 seed based on a BIP-39 mnemonic.
func NewSeedFromMnemonic(mnemonic string) ([]byte, error) {
	if mnemonic == "" {
		return nil, fmt.Errorf("mnemonic is required")
	}
	return bip39.NewSeedWithErrorChecking(mnemonic, "")
}

func MnemonicToEntropy(mnemonic string) ([]byte, error) {
	return bip39.MnemonicToByteArray(mnemonic, true)
}

func EntropyToMnemonic(entropy []byte) (string, error) {
	return bip39.NewMnemonic(entropy)
}

func RandomEntropy(bitSize ...int) ([]byte, error) {
	if len(bitSize) > 0 {
		return bip39.NewEntropy(bitSize[0])
	} else {
		return bip39.NewEntropy(EntropyBitSize12WordMnemonic) // default
	}
}

// RandomSeed returns a randomly generated BIP-39 seed.
func RandomSeed() ([]byte, error) {
	b := make([]byte, 64)
	_, err := rand.Read(b)
	return b, err
}

func IsValidMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

// ParseDerivationPath parses the derivation path in string format into []uint32
func ParseDerivationPath(path string) (accounts.DerivationPath, error) {
	return accounts.ParseDerivationPath(path)
}

func (h *HDNode) Mnemonic() string {
	return h.mnemonic
}

func (h *HDNode) Entropy() []byte {
	return h.entropy
}

func (h *HDNode) DerivationPath() accounts.DerivationPath {
	return h.derivationPath
}

func (h *HDNode) Address() common.Address {
	return h.address
}

func (h *HDNode) PrivateKey() *ecdsa.PrivateKey {
	return h.privateKey
}

func (h *HDNode) PublicKey() *ecdsa.PublicKey {
	return h.publicKey
}

func (h *HDNode) DerivePathFromString(path string) error {
	derivationPath, err := ParseDerivationPath(path)
	if err != nil {
		return err
	}
	return h.DerivePath(derivationPath)
}

func (h *HDNode) DerivePath(derivationPath accounts.DerivationPath) error {
	privateKey, err := derivePrivateKey(h.masterKey, derivationPath)
	if err != nil {
		return err
	}
	publicKey, err := derivePublicKey(h.masterKey, derivationPath)
	if err != nil {
		return err
	}
	address, err := deriveAddress(h.masterKey, derivationPath)
	if err != nil {
		return err
	}

	h.derivationPath = derivationPath
	h.privateKey = privateKey
	h.publicKey = publicKey
	h.address = address

	return nil
}

func (h *HDNode) DeriveAccountIndex(accountIndex uint32) error {
	x := len(h.derivationPath)
	if x < 4 {
		return fmt.Errorf("invalid account derivation path")
	}

	// copy + update
	updatedPath := make(accounts.DerivationPath, len(h.derivationPath))
	copy(updatedPath, h.derivationPath)
	updatedPath[x-1] = accountIndex

	return h.DerivePath(updatedPath)
}

func (h *HDNode) Clone() (*HDNode, error) {
	derivationPath := make(accounts.DerivationPath, len(h.derivationPath))
	copy(derivationPath, h.derivationPath)
	return NewHDNodeFromMnemonic(h.Mnemonic(), &derivationPath)
}

// DerivePrivateKey derives the private key of the derivation path.
func derivePrivateKey(masterKey *hdkeychain.ExtendedKey, path accounts.DerivationPath) (*ecdsa.PrivateKey, error) {
	var err error
	key := masterKey
	for _, n := range path {
		key, err = key.Derive(n)
		if err != nil {
			return nil, err
		}
	}

	privateKey, err := key.ECPrivKey()
	if err != nil {
		return nil, err
	}

	privateKeyECDSA := privateKey.ToECDSA()
	return privateKeyECDSA, nil
}

// DerivePublicKey derives the public key of the derivation path.
func derivePublicKey(masterKey *hdkeychain.ExtendedKey, path accounts.DerivationPath) (*ecdsa.PublicKey, error) {
	privateKeyECDSA, err := derivePrivateKey(masterKey, path)
	if err != nil {
		return nil, err
	}

	publicKey := privateKeyECDSA.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("failed to get public key")
	}

	return publicKeyECDSA, nil
}

// DeriveAddress derives the account address of the derivation path.
func deriveAddress(masterKey *hdkeychain.ExtendedKey, path accounts.DerivationPath) (common.Address, error) {
	publicKeyECDSA, err := derivePublicKey(masterKey, path)
	if err != nil {
		return common.Address{}, err
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA)
	return address, nil
}
