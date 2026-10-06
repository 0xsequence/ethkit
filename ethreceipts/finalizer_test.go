package ethreceipts

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func TestHardeningSubscribeBeforeRunFinality(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 2)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, DefaultOptions, b, hardeningBlock(101))
	s := l.Subscribe(FilterTxnHash(tx.Hash())).(*subscriber)
	defer s.Unsubscribe()
	hardeningStart(t, l)
	if r := hardeningRead(t, s); r.Final {
		t.Fatal("receipt final before network threshold")
	}
	processed := make(chan struct{})
	s.AddFilter(FilterLogs(func([]*types.Log) bool { return false }).SearchCache(true).QueryOnChain(func(context.Context) (*types.Receipt, error) { close(processed); return nil, nil }))
	select {
	case <-processed:
	case <-time.After(time.Second):
		t.Fatal("registration did not complete")
	}
	hardeningNoReceipt(t, s)
	if len(s.Filters()) != 2 {
		t.Fatal("premature finality removed transaction filter")
	}
}

func TestHardeningFinalizerOrderAndOwners(t *testing.T) {
	tx1, _, _ := hardeningTxn(t, 4)
	tx2, _, _ := hardeningTxn(t, 5)
	newer, older := hardeningBlock(101, tx1), hardeningBlock(100, tx2)
	p := &hardeningProvider{receipt: func(_ context.Context, h common.Hash) (*types.Receipt, error) {
		if h == tx1.Hash() {
			return hardeningReceipt(newer, tx1), nil
		}
		return hardeningReceipt(older, tx2), nil
	}}
	l := hardeningListener(t, p, hardeningOptions())
	q1 := FilterLogs(func([]*types.Log) bool { return true }).ID(7).Finalize(true)
	q2 := FilterLogs(func([]*types.Log) bool { return true }).ID(7).Finalize(true)
	s := l.Subscribe(q1, q2).(*subscriber)
	defer s.Unsubscribe()
	for _, r := range []*types.Receipt{hardeningReceipt(newer, tx1), hardeningReceipt(older, tx2)} {
		if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: r}}); err != nil {
			t.Fatal(err)
		}
		hardeningRead(t, s)
		hardeningRead(t, s)
	}
	for _, tc := range []struct {
		head int64
		tx   *types.Transaction
		b    *ethmonitor.Block
	}{{103, tx2, older}, {104, tx1, newer}} {
		if err := s.finalizeReceipts(big.NewInt(tc.head)); err != nil {
			t.Fatal(err)
		}
		owners := map[Filterer]bool{}
		for i := 0; i < 2; i++ {
			select {
			case r := <-s.TransactionReceipt():
				if !r.Final || r.TransactionHash() != tc.tx.Hash() || r.BlockHash() != tc.b.Hash() {
					t.Errorf("wrong final at %d: %+v", tc.head, r)
				}
				owners[r.Filter] = true
			case <-time.After(100 * time.Millisecond):
				t.Errorf("missing owner final at %d", tc.head)
			}
		}
		if !owners[q1.(Filterer)] || !owners[q2.(Filterer)] {
			t.Errorf("finality lost distinct owners with same public ID at %d", tc.head)
		}
	}
	hardeningNoReceipt(t, s)
}

func TestHardeningExhaustedOwnerKeepsQueuedFinal(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 17)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true).MaxWait(1)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	hardeningProcess(t, l, s, b)
	hardeningRead(t, s)
	// Exhaustion removes matching and closes its signal, but documented pending
	// finality remains owned. Exercise that state without relying on a live chain.
	s.mu.Lock()
	s.filters = nil
	s.mu.Unlock()
	q.(*filter).closeExhausted()
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	final := hardeningRead(t, s)
	if !final.Final || final.Filter != q || final.BlockHash() != b.Hash() {
		t.Fatal("exhaustion canceled a queued final")
	}
	hardeningNoReceipt(t, s)
}

func TestHardeningClearCancelsExhaustedFinality(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 18)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	hardeningProcess(t, l, s, b)
	hardeningRead(t, s)
	s.mu.Lock()
	s.filters = nil
	s.mu.Unlock()
	q.(*filter).closeExhausted()
	s.ClearFilters()
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	hardeningNoReceipt(t, s)
}
