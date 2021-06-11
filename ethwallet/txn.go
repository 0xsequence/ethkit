package ethwallet

import (
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum/common"
)

// TODO: move this to ethtx package

type TransactionRequest struct {
	// To is the recipient address, can be account, contract or nil. If `to` is nil, it will assume contract creation
	To *common.Address

	// Nonce is the nonce of the transaction for the sender. If this value is left empty (nil), it will
	// automatically be assigned.
	Nonce *big.Int

	// GasLimit is the total gas the transaction is expected the consume. If this value is left empty (0), it will
	// automatically be estimated and assigned.
	GasLimit uint64

	// GasPrice (in WEI) offering to pay for per unit of gas. If this value is left empty (nil), it will
	// automatically be sampled and assigned.
	GasPrice *big.Int

	// ETHValue (in WEI) amount of ETH currency to send with this transaction. Optional.
	ETHValue *big.Int

	// Data is calldata / input when calling or creating a contract. Optional.
	Data []byte
}
