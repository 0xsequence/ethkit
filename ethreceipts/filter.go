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

type FilterTxnHash struct {
	TxnHash      common.Hash
	FetchReceipt bool
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
