package ethreceipts

import (
	"context"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func Filterr() {

}

type FilterCond struct { // or FilterQuery ..
	// ......
}

type Filter interface {
	Match(ctx context.Context, receipt Receipt) (bool, error)
}

// TODO: we could embed this in each below..
// type FilterOpts struct {
// 	FetchReceipt   bool
// 	Once           bool
// 	Finalize bool
//  RecentOnly bool
// }

type FilterTxnHash struct {
	TxnHash      common.Hash
	FetchReceipt bool
	// Once         bool // TODO: maybe dont event want this..?
	Finalize   bool
	RecentOnly bool // default will check retention history.. or, FromBlock .. 0 will be head, etc.

	MaxNumBlocksWait int // some max number of blocks to find a match until we unsubscribe..
	// this could also be set on ReceiptsListener options.. and override to be unlimited, etc..
}

func (f FilterTxnHash) Match(ctx context.Context, receipt Receipt) (bool, error) {
	ok := receipt.Hash() == f.TxnHash
	return ok, nil
}

type FilterFrom struct {
	From common.Address
}

func (f FilterFrom) Match(ctx context.Context, receipt Receipt) (bool, error) {
	ok := receipt.Message.From() == f.From
	return ok, nil
}

type FilterTo struct {
	To common.Address
}

func (f FilterTo) Match(ctx context.Context, receipt Receipt) (bool, error) {
	ok := *receipt.Message.To() == f.To
	return ok, nil
}

type FilterEventSig struct {
	EventSig common.Hash // event signature / topic
}

func (f FilterEventSig) Match(ctx context.Context, receipt Receipt) (bool, error) {
	// .... decode..?
	return false, nil
}

type FilterContractFromLog struct {
	ContractAddress common.Address
}

func (f FilterContractFromLog) Match(ctx context.Context, receipt Receipt) (bool, error) {
	for _, log := range receipt.Logs {
		if log.Address == f.ContractAddress {
			return true, nil
		}
	}
	return false, nil
}

type FilterEvent struct {
	Log func(*types.Log) bool
}

func (f FilterEvent) Match(ctx context.Context, receipt Receipt) (bool, error) {
	for _, log := range receipt.Logs {
		ok := f.Log(log)
		if ok {
			return true, nil
		}
	}
	return false, nil
}
