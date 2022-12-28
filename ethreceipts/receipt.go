package ethreceipts

import (
	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Receipt struct {
	*types.Transaction // lower case this...
	*types.Receipt
	Logs    []types.Log
	Message types.Message // TOOD: this intermediate type is lame.. .. unexport
	Removed bool          // reorged txn
	Final   bool          // flags that this receipt is finalized
	Filter  Filterer      // reference to filter which triggered this event
}

func (r *Receipt) FilterID() uint64 {
	if r.Filter != nil && r.Filter.Options().ID > 0 {
		return r.Filter.FilterID()
	} else {
		return 0
	}
}

func (r *Receipt) TransactionHash() ethkit.Hash {
	if r.Transaction != nil {
		return r.Transaction.Hash()
	} else if r.Receipt != nil {
		return r.Receipt.TxHash
	} else {
		return ethkit.Hash{}
	}
}
