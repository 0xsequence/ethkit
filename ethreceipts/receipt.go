package ethreceipts

import (
	"math/big"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Receipt struct {
	Receipt *types.Receipt
	Logs    []types.Log
	Removed bool     // reorged txn
	Final   bool     // flags that this receipt is finalized
	Filter  Filterer // reference to filter which triggered this event

	transaction *types.Transaction
	message     *types.Message // TOOD: this intermediate type is lame.. with new ethrpc we can remove
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
	} else if r.Receipt != nil {
		return r.Receipt.TxHash
	} else {
		return ethkit.Hash{}
	}
}

func (r *Receipt) From() common.Address {
	if r.Receipt != nil {
		return r.Receipt.From
	} else if r.message != nil {
		return r.message.From()
	} else {
		return common.Address{}
	}
}

func (r *Receipt) To() common.Address {
	if r.Receipt != nil {
		return r.Receipt.To
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

func (r *Receipt) Status() uint64 {
	return r.Receipt.Status
}

func (r *Receipt) BlockNumber() *big.Int {
	return r.Receipt.BlockNumber
}

func (r *Receipt) BlockHash() ethkit.Hash {
	return r.Receipt.BlockHash
}
