package ethreceipts

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Receipt struct {
	Filter  Filterer // reference to filter which triggered this event
	Final   bool     // flags that this receipt is finalized
	Reorged bool     // chain reorged / removed the txn

	chainID     *big.Int
	transaction *types.Transaction
	receipt     *types.Receipt
	logs        []*types.Log

	// TODOXXX: this intermediate type is lame.. with new ethrpc we can remove
	// NOTE: we only use this for From/To address resolution currently
	message atomic.Value
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
	} else {
		if msg, _ := r.AsMessage(); msg != nil {
			return msg.From
		}
	}
	return common.Address{}
}

func (r *Receipt) To() common.Address {
	if r.receipt != nil {
		return r.receipt.To
	} else {
		if msg, _ := r.AsMessage(); msg != nil {
			to := msg.To
			if to == nil {
				return common.Address{}
			} else {
				return *to
			}
		}
	}
	return common.Address{}
}

func (r *Receipt) AsMessage() (*core.Message, error) {
	msg, ok := r.message.Load().(*core.Message)
	if !ok {
		return nil, fmt.Errorf("ethreceipts: Receipt.message type-assertion fail, unexpected")
	}
	if msg != nil {
		return msg, nil
	}

	// TODOXXX: avoid using AsMessage as its fairly expensive operation, especially
	// to do it for every txn for every filter.
	// TODO: in order to do this, we'll have to update ethrpc with a different
	// implementation to just use raw types, aka, ethrpc/types.go with Block/Transaction/Receipt/Log ..
	txnMsg, err := ethtxn.AsMessage(r.transaction, r.chainID)
	if err != nil {
		// NOTE: this should never happen, but lets log in case it does. In the
		// future, we should just not use go-ethereum for these types.
		// l.log.Warn(fmt.Sprintf("unexpected failure of txn (%s index %d) on block %d (total txns=%d) AsMessage(..): %s",
		// 	txn.Hash(), i, block.NumberU64(), len(block.Transactions()), err,
		// ))
		return nil, err
	}
	r.message.Store(txnMsg)
	return txnMsg, nil
}
