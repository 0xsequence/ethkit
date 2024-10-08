package ethreceipts

import (
	"context"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// Filter the transaction payload for specific txn "hash"
func FilterTxnHash(txnHash ethkit.Hash) FilterQuery {
	return &filter{
		cond: FilterCond{
			TxnHash: ethkit.ToPtr(txnHash),
		},

		// default options for TxnHash filter. Note, other filter conds
		// have a different set of defaults.
		options: FilterOptions{
			Finalize:      true,
			LimitOne:      true,
			SearchCache:   true,
			SearchOnChain: true,

			// wait up to NumBlocksToFinality*2 number of blocks between
			// filter matches before unsubcribing if no matches occured
			MaxWait: ethkit.ToPtr(-1),
		},

		exhausted: make(chan struct{}),
	}
}

// Filter the transaction payload for "from" address.
func FilterFrom(from ethkit.Address) FilterQuery {
	return &filter{
		cond: FilterCond{
			From: ethkit.ToPtr(from),
		},

		// no default options for From filter
		options:   FilterOptions{},
		exhausted: make(chan struct{}),
	}
}

// Filter the transaction payload for "to" address.
func FilterTo(to ethkit.Address) FilterQuery {
	return &filter{
		cond: FilterCond{
			To: ethkit.ToPtr(to),
		},

		// no default options for To filter
		options:   FilterOptions{},
		exhausted: make(chan struct{}),
	}
}

// Filter the logs of a transaction and search for an event log
// from a specific contract address.
func FilterLogContract(contractAddress ethkit.Address) FilterQuery {
	return FilterLogs(func(logs []*types.Log) bool {
		for _, log := range logs {
			if log.Address == contractAddress {
				return true
			}
		}
		return false
	})
}

// Filter the log topics for a transaction
func FilterLogTopic(eventTopicHash ethkit.Hash) FilterQuery {
	return &filter{
		cond: FilterCond{
			LogTopic: ethkit.ToPtr(eventTopicHash),
		},

		// no default options for EventSig filter
		options:   FilterOptions{},
		exhausted: make(chan struct{}),
	}
}

// Filter logs of a transaction
func FilterLogs(logFn func([]*types.Log) bool) FilterQuery {
	return &filter{
		cond: FilterCond{
			Logs: logFn,
		},

		// no default options for Log filter
		options:   FilterOptions{},
		exhausted: make(chan struct{}),
	}
}

type Filterer interface {
	FilterQuery

	FilterID() uint64
	Options() FilterOptions
	Cond() FilterCond

	Match(ctx context.Context, receipt Receipt) (bool, error)
	StartBlockNum() uint64
	LastMatchBlockNum() uint64
	Exhausted() <-chan struct{}
}

type FilterQuery interface {
	ID(uint64) FilterQuery
	Finalize(bool) FilterQuery
	LimitOne(bool) FilterQuery
	SearchCache(bool) FilterQuery
	SearchOnChain(bool) FilterQuery
	MaxWait(int) FilterQuery
}

type FilterOptions struct {
	// ..
	ID uint64

	// ..
	Finalize bool

	// .
	LimitOne bool

	// ..
	SearchCache bool

	// SearchOnChain will search for txn hash on-chain. This is only useful
	// when used in combination with TxnHash filter cond.
	SearchOnChain bool

	// MaxWait filter option waits some number of blocks without a filter match after
	// which point will auto-unsubscribe the filter. This is useful to help automatically
	// remove filters which likely won't come up.
	//
	// nil : use the ReceiptsListener option FilterMaxWaitNumBlocks value as the default
	// -1  : set value to ReceiptsListener option NumFinality * 3
	// 0   : option is disabled, and has no limit on wait. filters need to be manually unsubscribed
	// N   : a specified number of blocks without a match before unsusbcribe
	MaxWait *int
}

type FilterCond struct {
	TxnHash  *ethkit.Hash
	From     *ethkit.Address
	To       *ethkit.Address
	LogTopic *ethkit.Hash // event signature topic hash
	Logs     func([]*types.Log) bool
}

type filter struct {
	options FilterOptions
	cond    FilterCond

	// startBlockNum is the first block number observed once filter is active
	startBlockNum uint64

	// lastMatchBlockNum is the block number where a last match occured
	lastMatchBlockNum uint64

	// exhausted signals if the filter hit MaxWait
	exhausted chan struct{}
}

var (
	_ Filterer    = &filter{}
	_ FilterQuery = &filter{}
)

func (f *filter) ID(id uint64) FilterQuery {
	f.options.ID = id
	return f
}

func (f *filter) Finalize(finalize bool) FilterQuery {
	f.options.Finalize = finalize
	return f
}

func (f *filter) LimitOne(limitOne bool) FilterQuery {
	f.options.LimitOne = limitOne
	return f
}

func (f *filter) SearchCache(searchCache bool) FilterQuery {
	f.options.SearchCache = searchCache
	return f
}

func (f *filter) SearchOnChain(searchOnChain bool) FilterQuery {
	f.options.SearchOnChain = searchOnChain
	return f
}

func (f *filter) MaxWait(maxWait int) FilterQuery {
	f.options.MaxWait = &maxWait
	return f
}

func (f *filter) FilterID() uint64 {
	return f.options.ID
}

func (f *filter) Options() FilterOptions {
	return f.options
}

func (f *filter) Cond() FilterCond {
	return f.cond
}

func (f *filter) Match(ctx context.Context, receipt Receipt) (bool, error) {
	c := f.cond

	if c.TxnHash != nil {
		ok := receipt.TransactionHash() == *c.TxnHash
		return ok, nil
	}

	if c.From != nil {
		ok := receipt.From() == *c.From
		return ok, nil
	}

	if c.To != nil {
		ok := receipt.To() == *c.To
		return ok, nil
	}

	if c.LogTopic != nil {
		for _, log := range receipt.Logs() {
			if len(log.Topics) == 0 {
				continue
			}
			if *c.LogTopic == log.Topics[0] {
				return true, nil
			}
		}
		return false, nil
	}

	if c.Logs != nil {
		ok := c.Logs(receipt.Logs())
		return ok, nil
	}

	return false, ErrFilterCond
}

func (f *filter) StartBlockNum() uint64 {
	return f.startBlockNum
}

func (f *filter) LastMatchBlockNum() uint64 {
	return f.lastMatchBlockNum
}

func (f *filter) Exhausted() <-chan struct{} {
	return f.exhausted
}
