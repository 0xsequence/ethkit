package ethreceipts

import (
	"context"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Receipt struct {
	*types.Transaction
	*types.Receipt
	Message types.Message // TOOD: this is lame..
}

type Subscription interface {
	TransactionReceipt() <-chan Receipt
	Done() <-chan struct{}
	Unsubscribe()

	// TODO: add..
	Filters() []Filter
	// AddFilter(f any)
	// RemoveFilter(f any)
}

type Filter interface {
	Match(ctx context.Context, receipt Receipt) (bool, error)
}

type FilterTxnHash struct {
	TxnHash common.Hash
}

func (f FilterTxnHash) Match(ctx context.Context, receipt Receipt) (bool, error) {
	return false, nil
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

type FilterLog struct {
	Log func(*types.Log) bool
}

func (f FilterLog) Match(ctx context.Context, receipt Receipt) (bool, error) {
	return false, nil
}

var _ Subscription = &subscriber{}

type subscriber struct {
	ch          <-chan Receipt
	sendCh      chan<- Receipt
	done        chan struct{}
	unsubscribe func()

	filters []Filter
}

func (s *subscriber) TransactionReceipt() <-chan Receipt {
	return s.ch
}

func (s *subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *subscriber) Unsubscribe() {
	s.unsubscribe()
}

func (s *subscriber) Filters() []Filter {
	return s.filters
}
