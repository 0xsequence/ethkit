package ethwallet

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
)

// Validate the public key address of a signed message
func ValidateEthereumSignature(address string, message string, signature string) (bool, error) {
	if !common.IsHexAddress(address) {
		return false, errors.Errorf("address is not a valid Ethereum address")
	}
	if len(message) < 1 || len(signature) < 1 {
		return false, errors.Errorf("message and signature must not be empty")
	}
	if len(message) > 100 || len(signature) > 150 {
		return false, errors.Errorf("message and signature exceed size limit")
	}
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%v%v", len(message), message)
	sig, err := hexutil.Decode(signature)
	if err != nil {
		return false, errors.Errorf("signature is an invalid hex string")
	}
	if len(sig) != 65 {
		return false, errors.Errorf("signature is not of proper length")
	}
	hash := crypto.Keccak256([]byte(msg))
	sig[64] -= 27 // recovery ID

	pubkey, err := crypto.SigToPub(hash, sig)
	if err != nil {
		return false, err
	}
	key := crypto.PubkeyToAddress(*pubkey).Hex()
	if strings.ToLower(key) == strings.ToLower(address) {
		return true, nil
	}
	return false, errors.Errorf("invalid signature")
}
