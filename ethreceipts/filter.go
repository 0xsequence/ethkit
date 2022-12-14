package ethreceipts

import (
	"context"
	"fmt"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Filter interface {
	// Options() FilterOpts
	Match(ctx context.Context, receipt Receipt) (bool, error)
}

func FilterTxnHash(txnHash ethkit.Hash) *FilterCond {
	return &FilterCond{
		TxnHash: ethkit.PtrTo(txnHash),

		// default options for TxnHash filter
		FilterOpts: FilterOpts{
			Finalize:     true,
			MatchAndDone: true,
			SearchCache:  true,
		},
	}
}

func FilterFrom(from ethkit.Address) *FilterCond {
	return &FilterCond{
		From: ethkit.PtrTo(from),

		// no default options for From filter
		FilterOpts: FilterOpts{},
	}
}

func FilterTo(to ethkit.Address) *FilterCond {
	return &FilterCond{
		From: ethkit.PtrTo(to),

		// no default options for To filter
		FilterOpts: FilterOpts{},
	}
}

func FilterEventSig(eventSig ethkit.Hash) *FilterCond {
	return &FilterCond{
		EventSig: ethkit.PtrTo(eventSig),

		// no default options for EventSig filter
		FilterOpts: FilterOpts{},
	}
}

func FilterLog(logFn func(*types.Log) bool) *FilterCond {
	return &FilterCond{
		Log: logFn,

		// no default options for Log filter
		FilterOpts: FilterOpts{},
	}
}

type FilterCond struct {
	FilterOpts
	TxnHash  *ethkit.Hash
	From     *ethkit.Address
	To       *ethkit.Address
	EventSig *ethkit.Hash // event signature / topic
	Log      func(*types.Log) bool
}

type FilterOpts struct {
	ID                 uint64 // TODO: finish
	Finalize           bool
	MatchAndDone       bool // TODO: hook up
	SearchCache        bool // TODO: hook up
	MaxNumBlocksListen int  // TODO: rename / hook up..
}

func (c *FilterCond) ID(id uint64) *FilterCond {
	c.FilterOpts.ID = id
	return c
}

func (c *FilterCond) Finalize(finalize bool) *FilterCond {
	c.FilterOpts.Finalize = true
	return c
}

func (c *FilterCond) MatchAndDone(matchAndDone bool) *FilterCond {
	c.FilterOpts.MatchAndDone = true
	return c
}

func (c *FilterCond) SearchCache(searchCache bool) *FilterCond {
	c.FilterOpts.SearchCache = true
	return c
}

func (c *FilterCond) Match(ctx context.Context, receipt Receipt) (bool, error) {
	if c.TxnHash != nil {
		ok := receipt.Hash() == *c.TxnHash
		return ok, nil
	}

	if c.From != nil {
		// TODO: check if receipt.Message is set..
		ok := receipt.Message.From() == *c.From
		return ok, nil
	}

	if c.To != nil {
		// TODO: check if receipt.Message is set..
		ok := *receipt.Message.To() == *c.To
		return ok, nil
	}

	if c.EventSig != nil {
		// TODO: implement..
		return false, nil
	}

	if c.Log != nil {
		for _, log := range receipt.Logs {
			ok := c.Log(log)
			if ok {
				return true, nil
			}
		}
		return false, nil
	}

	return false, fmt.Errorf("missing filter condition")
}

/*
// TODO: rename to Matcher or filterer ..
type Filter interface {
	Match(ctx context.Context, receipt Receipt) (bool, error)
}

type FilterOpts struct {
	ID           uint64
	Finalize     bool
	MatchAndDone bool

	Once       bool
	RecentOnly bool
}

type FilterTxnHash struct {
	TxnHash common.Hash

	ID           uint64 // optional, will be assigned to any output..
	FetchReceipt bool
	// Once         bool // TODO: maybe dont event want this..?
	Finalize   bool
	RecentOnly bool // default will check retention history.. or, FromBlock .. 0 will be head, etc.

	MaxNumBlocksListen int // some max number of blocks to find a match until we unsubscribe..
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
*/
