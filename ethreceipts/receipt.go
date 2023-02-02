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
	if r.receipt != nil {
		return r.receipt.Status
	} else {
		return 0
	}
}

func (r *Receipt) BlockNumber() *big.Int {
	if r.receipt != nil {
		return r.receipt.BlockNumber
	} else {
		return nil
	}
}

func (r *Receipt) BlockHash() ethkit.Hash {
	if r.receipt != nil {
		return r.receipt.BlockHash
	} else {
		return ethkit.Hash{}
	}
}

func (r *Receipt) Type() uint8 {
	if r.receipt != nil {
		return r.receipt.Type
	} else {
		return 0
	}
}

func (r *Receipt) Root() []byte {
	if r.receipt != nil {
		return r.receipt.PostState
	} else {
		return nil
	}
}

func (r *Receipt) Bloom() types.Bloom {
	if r.receipt != nil {
		return r.receipt.Bloom
	} else {
		return types.Bloom{}
	}
}

func (r *Receipt) TransactionIndex() uint {
	if r.receipt != nil {
		return r.receipt.TransactionIndex
	} else {
		return 0
	}
}

// DeployedContractAddress returns the address if this receipt is related to
// a contract deployment.
func (r *Receipt) DeployedContractAddress() common.Address {
	if r.receipt != nil {
		return r.receipt.ContractAddress
	} else {
		return common.Address{}
	}
}

func (r *Receipt) CumulativeGasUsed() uint64 {
	if r.receipt != nil {
		return r.receipt.CumulativeGasUsed
	} else {
		return 0
	}
}

func (r *Receipt) EffectiveGasPrice() *big.Int {
	if r.receipt != nil {
		return r.receipt.EffectiveGasPrice
	} else {
		return nil
	}
}

func (r *Receipt) GasUsed() uint64 {
	if r.receipt != nil {
		return r.receipt.GasUsed
	} else {
		return 0
	}
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
