package ethreceipts

import (
	"math/big"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Receipt struct {
	Filter  Filterer // reference to filter which triggered this event
	Final   bool     // flags that this receipt is finalized
	Reorged bool     // chain reorged / removed the txn

	transaction *types.Transaction
	message     *types.Message // TOOD: this intermediate type is lame.. with new ethrpc we can remove
	receipt     *types.Receipt
	logs        []*types.Log
}

func (r *Receipt) Receipt() *types.Receipt {
	return r.receipt
}

func (r *Receipt) FilterID() uint64 {
	if r.Filter != nil && r.Filter.Options().ID > 0 {
		return r.Filter.FilterID()
	} else {
		return 0
	}
}

func (r *Receipt) TransactionHash() ethkit.Hash {
	if r.transaction != nil {
		return r.transaction.Hash()
	} else if r.receipt != nil {
		return r.receipt.TxHash
	} else {
		return ethkit.Hash{}
	}
}

func (r *Receipt) Status() uint64 {
	return r.receipt.Status
}

func (r *Receipt) BlockNumber() *big.Int {
	return r.receipt.BlockNumber
}

func (r *Receipt) BlockHash() ethkit.Hash {
	return r.receipt.BlockHash
}

func (r *Receipt) Type() uint8 {
	return r.receipt.Type
}

func (r *Receipt) Root() []byte {
	return r.receipt.PostState
}

func (r *Receipt) Bloom() types.Bloom {
	return r.receipt.Bloom
}

func (r *Receipt) TransactionIndex() uint {
	return r.receipt.TransactionIndex
}

// DeployedContractAddress returns the address if this receipt is related to
// a contract deployment.
func (r *Receipt) DeployedContractAddress() common.Address {
	return r.receipt.ContractAddress
}

func (r *Receipt) CumulativeGasUsed() uint64 {
	return r.receipt.CumulativeGasUsed
}

func (r *Receipt) EffectiveGasPrice() *big.Int {
	return r.receipt.EffectiveGasPrice
}

func (r *Receipt) GasUsed() uint64 {
	return r.receipt.GasUsed
}

func (r *Receipt) Logs() []*types.Log {
	if r.receipt != nil && len(r.receipt.Logs) > 0 {
		return r.receipt.Logs
	} else {
		return r.logs
	}
}

func (r *Receipt) From() common.Address {
	if r.receipt != nil {
		return r.receipt.From
	} else if r.message != nil {
		return r.message.From()
	} else {
		return common.Address{}
	}
}

func (r *Receipt) To() common.Address {
	if r.receipt != nil {
		return r.receipt.To
	} else if r.message != nil {
		to := r.message.To()
		if to == nil {
			return common.Address{}
		} else {
			return *to
		}
	} else {
		return common.Address{}
	}
}
