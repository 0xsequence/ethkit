package ethreceipts

import (
	"context"
	"fmt"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Filter interface {
	GetID() uint64
	Options() FilterOpts
	Match(ctx context.Context, receipt Receipt) (bool, error)
}

func FilterTxnHash(txnHash ethkit.Hash) *FilterCond {
	return &FilterCond{
		TxnHash: ethkit.PtrTo(txnHash),

		// default options for TxnHash filter
		FilterOpts: FilterOpts{
			Finalize:      true,
			LimitOne:      true,
			SearchHistory: true,
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
	ID       uint64
	Finalize bool

	LimitOne      bool
	SearchHistory bool
	// TODO: perhaps we should have SearchCache and SearchHistory ..? and SearchHistory will do a FetchReceipt from block 0..?
	MaxNumBlocksListen int // TODO: rename / hook up..
}

func (c *FilterCond) ID(id uint64) *FilterCond {
	c.FilterOpts.ID = id
	return c
}

func (c *FilterCond) Finalize(finalize bool) *FilterCond {
	c.FilterOpts.Finalize = finalize
	return c
}

func (c *FilterCond) LimitOne(limitOne bool) *FilterCond {
	c.FilterOpts.LimitOne = limitOne
	return c
}

func (c *FilterCond) SearchHistory(searchCache bool) *FilterCond {
	c.FilterOpts.SearchHistory = searchCache
	return c
}

func (c *FilterCond) GetID() uint64 {
	return c.FilterOpts.ID
}

func (c *FilterCond) Options() FilterOpts {
	return c.FilterOpts
}

func (c *FilterCond) Match(ctx context.Context, receipt Receipt) (bool, error) {
	if c.TxnHash != nil {
		ok := receipt.Hash() == *c.TxnHash
		return ok, nil
	}

	if c.From != nil {
		ok := receipt.Message.From() == *c.From
		return ok, nil
	}

	if c.To != nil {
		ok := *receipt.Message.To() == *c.To
		return ok, nil
	}

	if c.EventSig != nil {
		for _, log := range receipt.Logs {
			if len(log.Topics) == 0 {
				continue
			}
			if *c.EventSig == log.Topics[0] {
				return true, nil
			}
		}
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
