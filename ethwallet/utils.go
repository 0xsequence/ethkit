package ethwallet

import (
	"bytes"
	"fmt"

	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
)

func RecoverAddress(message, signature []byte) (common.Address, error) {
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%v%s", len(message), message)
	if len(signature) != 65 {
		return common.Address{}, fmt.Errorf("signature is not of proper length")
	}
	return RecoverAddressFromDigest(crypto.Keccak256([]byte(msg)), signature)
}

func RecoverAddressFromDigest(digest, signature []byte) (common.Address, error) {
	if len(digest) != 32 {
		return common.Address{}, fmt.Errorf("digest is not of proper length (=32)")
	}
	if len(signature) != 65 {
		return common.Address{}, fmt.Errorf("signature is not of proper length (=65)")
	}

	sig := make([]byte, 65)
	copy(sig, signature)

	if sig[64] > 1 {
		sig[64] -= 27 // recovery ID
	}

	pubkey, err := crypto.SigToPub(digest, sig)
	if err != nil {
		return common.Address{}, err
	}
	address := crypto.PubkeyToAddress(*pubkey)

	return address, nil
}

func IsValidEOASignature(address common.Address, digest, signature []byte) (bool, error) {
	if len(digest) == 0 || len(signature) == 0 {
		return false, fmt.Errorf("digest and signature must not be empty")
	}
	if len(signature) != 65 {
		return false, fmt.Errorf("signature is not of proper length")
	}

	sig := make([]byte, 65)
	copy(sig, signature)

	if sig[64] > 1 {
		sig[64] -= 27 // recovery ID
	}

	pubkey, err := crypto.SigToPub(digest, sig)
	if err != nil {
		return false, err
	}
	recoveredAddress := crypto.PubkeyToAddress(*pubkey)
	if recoveredAddress == address {
		return true, nil
	}
	return false, fmt.Errorf("invalid signature")
}

func IsValid191Signature(address common.Address, message, signature []byte) (bool, error) {
	if len(message) == 0 || len(signature) == 0 {
		return false, fmt.Errorf("message and signature must not be empty")
	}
	if len(signature) != 65 {
		return false, fmt.Errorf("signature is not of proper length")
	}

	message191 := []byte("\x19Ethereum Signed Message:\n")
	if !bytes.HasPrefix(message, message191) {
		mlen := fmt.Sprintf("%d", len(message))
		message191 = append(message191, []byte(mlen)...)
		message191 = append(message191, message...)
	} else {
		message191 = message
	}

	sig := make([]byte, 65)
	copy(sig, signature)

	hash := crypto.Keccak256(message191)
	if sig[64] > 1 {
		sig[64] -= 27 // recovery ID
	}

	pubkey, err := crypto.SigToPub(hash, sig)
	if err != nil {
		return false, err
	}
	recoveredAddress := crypto.PubkeyToAddress(*pubkey)
	if recoveredAddress == address {
		return true, nil
	}
	return false, fmt.Errorf("invalid signature")
}

// Validate the public key address of a signed message
func ValidateEthereumSignature(address string, message []byte, signatureHex string) (bool, error) {
	sig, err := ethcoder.HexDecode(signatureHex)
	if err != nil {
		return false, err
	}
	return IsValid191Signature(common.HexToAddress(address), message, sig)
}
