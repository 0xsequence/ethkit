package ethwallet

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// Validate the public key address of a signed message
func ValidateEthereumSignature(address string, message []byte, signatureHex string) (bool, error) {
	if !common.IsHexAddress(address) {
		return false, fmt.Errorf("address is not a valid Ethereum address")
	}
	if len(message) < 1 || len(signatureHex) < 1 {
		return false, fmt.Errorf("message and signature must not be empty")
	}
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%v%s", len(message), message)
	sig, err := hexutil.Decode(signatureHex)
	if err != nil {
		return false, fmt.Errorf("signature is an invalid hex string")
	}
	if len(sig) != 65 {
		return false, fmt.Errorf("signature is not of proper length")
	}
	hash := crypto.Keccak256([]byte(msg))
	if sig[64] > 1 {
		sig[64] -= 27 // recovery ID
	}

	pubkey, err := crypto.SigToPub(hash, sig)
	if err != nil {
		return false, err
	}
	key := crypto.PubkeyToAddress(*pubkey).Hex()
	if strings.ToLower(key) == strings.ToLower(address) {
		return true, nil
	}
	return false, fmt.Errorf("invalid signature")
}
