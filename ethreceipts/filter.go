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

		// default options for TxnHash filter. Note, other filter conds
		// have a different set of defaults.
		FilterOpts: FilterOpts{
			Finalize:      true,
			LimitOne:      true,
			SearchCache:   true,
			SearchOnChain: true,

			// wait up to NumBlocksToFinality*2 number of blocks between
			// filter matches before unsubcribing if no matches occured
			MaxWait: -1,
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

func FilterLogTopic(eventTopicHash ethkit.Hash) *FilterCond {
	return &FilterCond{
		LogTopic: ethkit.PtrTo(eventTopicHash),

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
	LogTopic *ethkit.Hash // event signature topic hash
	Log      func(*types.Log) bool
}

type FilterOpts struct {
	ID            uint64
	Finalize      bool
	LimitOne      bool
	SearchCache   bool
	SearchOnChain bool
	MaxWait       int
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

func (c *FilterCond) SearchCache(searchCache bool) *FilterCond {
	c.FilterOpts.SearchCache = searchCache
	return c
}

func (c *FilterCond) SearchOnChain(searchOnChain bool) *FilterCond {
	c.FilterOpts.SearchOnChain = searchOnChain
	return c
}

func (c *FilterCond) MaxWait(maxWait int) *FilterCond {
	c.FilterOpts.MaxWait = maxWait
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

	if c.LogTopic != nil && len(receipt.Logs) > 0 {
		for _, log := range receipt.Logs {
			if len(log.Topics) == 0 {
				continue
			}
			if *c.LogTopic == log.Topics[0] {
				return true, nil
			}
		}
		return false, nil
	}

	if c.Log != nil && len(receipt.Logs) > 0 {
		for _, log := range receipt.Logs {
			ok := c.Log(&log)
			if ok {
				return true, nil
			}
		}
		return false, nil
	}

	return false, fmt.Errorf("missing filter condition")
}
