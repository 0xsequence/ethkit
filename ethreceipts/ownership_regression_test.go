package ethreceipts

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func TestHardeningAddressFilters(t *testing.T) {
	tx, from, to := hardeningTxn(t, 3)
	b := hardeningBlock(100, tx)
	for _, tc := range []struct {
		name string
		q    FilterQuery
	}{{"from", FilterFrom(from)}, {"to", FilterTo(to)}} {
		t.Run(tc.name, func(t *testing.T) {
			p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
			l := hardeningListener(t, p, hardeningOptions(), b)
			s := l.Subscribe(tc.q.SearchCache(true))
			defer s.Unsubscribe()
			hardeningStart(t, l)
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx.Hash() || r.BlockHash() != b.Hash() {
					t.Fatalf("wrong matching transaction: %+v", r)
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("signed address filter missed cached transaction")
			}
		})
	}
}

func TestHardeningLimitOneSnapshots(t *testing.T) {
	for _, sameBlock := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_block", false: "retained_blocks"}[sameBlock], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 7)
			tx2, _, _ := hardeningTxn(t, 8)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			blocks := ethmonitor.Blocks{b1, b2}
			if sameBlock {
				b1 = hardeningBlock(100, tx1, tx2)
				b2 = b1
				blocks = ethmonitor.Blocks{b1}
			}
			p := &hardeningProvider{receipt: func(_ context.Context, h common.Hash) (*types.Receipt, error) {
				if h == tx1.Hash() {
					return hardeningReceipt(b1, tx1), nil
				}
				return hardeningReceipt(b2, tx2), nil
			}}
			l := hardeningListener(t, p, hardeningOptions())
			s := l.Subscribe(FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true)).(*subscriber)
			defer s.Unsubscribe()
			fs := s.Filters()
			if _, err := l.processCachedBlocks(context.Background(), blocks, []*subscriber{s}, [][]Filterer{fs}); err != nil {
				t.Fatal(err)
			}
			r := hardeningRead(t, s)
			if r.TransactionHash() != tx1.Hash() {
				t.Errorf("LimitOne did not select first match: %s", r.TransactionHash())
			}
			hardeningNoReceipt(t, s)
		})
	}
}

func TestHardeningConcurrentLimitOne(t *testing.T) {
	tx1, _, _ := hardeningTxn(t, 12)
	tx2, _, _ := hardeningTxn(t, 13)
	b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
	entered, release := make(chan struct{}), make(chan struct{})
	p := &hardeningProvider{receipt: func(ctx context.Context, h common.Hash) (*types.Receipt, error) {
		if h == tx1.Hash() {
			close(entered)
			select {
			case <-release:
				return hardeningReceipt(b1, tx1), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return hardeningReceipt(b2, tx2), nil
	}}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(true)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	fs := s.Filters()
	done := make(chan error, 1)
	go func() {
		_, err := l.processCachedBlocks(context.Background(), ethmonitor.Blocks{b1}, []*subscriber{s}, [][]Filterer{fs})
		done <- err
	}()
	<-entered
	if _, err := l.processCachedBlocks(context.Background(), ethmonitor.Blocks{b2}, []*subscriber{s}, [][]Filterer{fs}); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	r := hardeningRead(t, s)
	if r.TransactionHash() != tx1.Hash() || r.Filter != q {
		t.Errorf("concurrent LimitOne selected different transaction: %+v", r)
	}
	hardeningNoReceipt(t, s)
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	final := hardeningRead(t, s)
	if !final.Final || final.TransactionHash() != tx1.Hash() || final.Filter != q {
		t.Errorf("wrong claimed final: %+v", final)
	}
	hardeningNoReceipt(t, s)
}

func TestHardeningExplicitRemovalCancelsOnlyOwner(t *testing.T) {
	for _, clearAll := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove_one", true: "clear_all"}[clearAll], func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 16)
			b := hardeningBlock(100, tx)
			p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
			l := hardeningListener(t, p, hardeningOptions())
			q1 := FilterLogs(func([]*types.Log) bool { return true }).ID(1).Finalize(true)
			q2 := FilterLogs(func([]*types.Log) bool { return true }).ID(2).Finalize(true)
			s := l.Subscribe(q1, q2).(*subscriber)
			defer s.Unsubscribe()
			hardeningProcess(t, l, s, b)
			hardeningRead(t, s)
			hardeningRead(t, s)
			if clearAll {
				s.ClearFilters()
			} else {
				s.RemoveFilter(q1.(Filterer))
			}
			if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
				t.Fatal(err)
			}
			if !clearAll {
				r := hardeningRead(t, s)
				if !r.Final || r.Filter != q2 || r.BlockHash() != b.Hash() {
					t.Errorf("removal lost surviving owner: %+v", r)
				}
			}
			hardeningNoReceipt(t, s)
		})
	}
}

type ownershipValueFilter struct {
	Filterer
	labels []string
	match  func(context.Context, Receipt) (bool, error)
}

func (f ownershipValueFilter) Match(ctx context.Context, r Receipt) (bool, error) {
	return f.match(ctx, r)
}

func TestOwnershipCustomValueFilter(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 100)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, hardeningOptions(), b)
	base := FilterLogs(func([]*types.Log) bool { return false }).SearchCache(true).(Filterer)
	var matches atomic.Int32
	q := ownershipValueFilter{base, []string{"custom"}, func(_ context.Context, r Receipt) (bool, error) {
		matches.Add(1)
		return r.TransactionHash() == tx.Hash(), nil
	}}
	s := l.Subscribe(q)
	defer s.Unsubscribe()
	gotFilter, ok := s.Filters()[0].(ownershipValueFilter)
	if !ok || gotFilter.Filterer != base || gotFilter.labels[0] != "custom" {
		t.Fatal("public Filters lost original custom value")
	}
	hardeningStart(t, l)
	r := hardeningRead(t, s)
	original, ok := r.Filter.(ownershipValueFilter)
	if !ok || original.Filterer != base || original.labels[0] != "custom" || original.match == nil {
		t.Fatal("receipt lost original custom Filterer value")
	}
	if r.TransactionHash() != tx.Hash() || r.BlockHash() != b.Hash() || matches.Load() == 0 {
		t.Fatal("custom Match behavior lost")
	}
	s.ClearFilters()
}

type ownershipCollectionFilter struct {
	Filterer
	cancel context.CancelFunc
}

func (f *ownershipCollectionFilter) Match(context.Context, Receipt) (bool, error) {
	if f.cancel != nil {
		f.cancel()
		return false, nil
	}
	return false, errors.New("temporary match failure")
}

func TestOwnershipCollectionAbort(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "matcher_error", true: "cancellation"}[canceled], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 101)
			tx2, _, _ := hardeningTxn(t, 102)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true)
			bad := &ownershipCollectionFilter{Filterer: FilterLogs(func([]*types.Log) bool { return false }).(Filterer)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if canceled {
				bad.cancel = cancel
			}
			s := l.Subscribe(q, bad).(*subscriber)
			defer s.Unsubscribe()
			_, err := s.matchFiltersAndPublish(ctx, s.Filters(), []Receipt{{receipt: hardeningReceipt(b1, tx1)}})
			if err == nil {
				t.Error("expected matching abort")
			}
			if canceled && !errors.Is(err, context.Canceled) {
				t.Errorf("cancellation error: %v", err)
			}
			if len(s.inFlight) != 0 || len(s.claims) != 0 {
				t.Errorf("aborted collection retained work: inFlight=%d claims=%d", len(s.inFlight), len(s.claims))
			}
			hardeningNoReceipt(t, s)
			s.RemoveFilter(bad)
			if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b2, tx2)}}); err != nil {
				t.Fatal(err)
			}
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx2.Hash() || r.Filter != q {
					t.Error("unused claim blocked later transaction")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("aborted reservation blocked later processing")
			}
		})
	}
}

func TestOwnershipQueryReuse(t *testing.T) {
	for _, clearAll := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove", true: "clear"}[clearAll], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 103)
			tx2, _, _ := hardeningTxn(t, 104)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(true)
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b1, tx1)}}); err != nil {
				t.Fatal(err)
			}
			hardeningRead(t, s)
			if clearAll {
				s.ClearFilters()
			} else {
				s.RemoveFilter(q.(Filterer))
			}
			if len(s.claims) != 0 || len(s.inFlight) != 0 || len(s.deliveries) != 0 {
				t.Error("explicit removal retained canceled ownership")
			}
			s.AddFilter(q)
			if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b2, tx2)}}); err != nil {
				t.Fatal(err)
			}
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx2.Hash() || r.Filter != q || r.Final {
					t.Error("reused query kept old selection")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("reused LimitOne query missed second transaction")
			}
			if err := s.finalizeReceipts(big.NewInt(104)); err != nil {
				t.Fatal(err)
			}
			select {
			case r := <-s.TransactionReceipt():
				if !r.Final || r.TransactionHash() != tx2.Hash() || r.Filter != q {
					t.Error("canceled owner finalized in new lifetime")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("new owner did not finalize")
			}
			hardeningNoReceipt(t, s)
		})
	}
}

func TestOwnershipReaddRejectsOldWorker(t *testing.T) {
	for _, clearAll := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove", true: "clear"}[clearAll], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 105)
			tx2, _, _ := hardeningTxn(t, 106)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			entered, release := make(chan struct{}), make(chan struct{})
			p := &hardeningProvider{receipt: func(ctx context.Context, h common.Hash) (*types.Receipt, error) {
				if h == tx1.Hash() {
					close(entered)
					select {
					case <-release:
						return hardeningReceipt(b1, tx1), nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return hardeningReceipt(b2, tx2), nil
			}}
			l := hardeningListener(t, p, hardeningOptions())
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true)
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			done := make(chan error, 1)
			go func() {
				_, err := l.processCachedBlocks(context.Background(), ethmonitor.Blocks{b1}, []*subscriber{s}, [][]Filterer{s.Filters()})
				done <- err
			}()
			<-entered
			if clearAll {
				s.ClearFilters()
			} else {
				s.RemoveFilter(q.(Filterer))
			}
			s.AddFilter(q)
			hardeningProcess(t, l, s, b2)
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx2.Hash() || r.Filter != q {
					t.Error("new lifetime selected old transaction")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("new lifetime suppressed by old claim")
			}
			close(release)
			<-done
			hardeningNoReceipt(t, s)
			if len(s.claims) != 0 || len(s.inFlight) != 0 || len(s.deliveries) != 0 {
				t.Error("completed/canceled lifetime retained state")
			}
		})
	}
}

func TestOwnershipCompletionReleasesState(t *testing.T) {
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	s := l.Subscribe().(*subscriber)
	defer s.Unsubscribe()
	for i := 0; i < 32; i++ {
		tx, _, _ := hardeningTxn(t, uint64(200+i))
		b := hardeningBlock(100, tx)
		q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(i%4 != 0)
		s.AddFilter(q)
		if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b, tx)}}); err != nil {
			t.Fatal(err)
		}
		hardeningRead(t, s)
		switch i % 4 {
		case 1:
			if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
				t.Fatal(err)
			}
			hardeningRead(t, s)
		case 2:
			s.RemoveFilter(q.(Filterer))
		case 3:
			s.ClearFilters()
		}
		if len(s.Filters()) != 0 || len(s.claims) != 0 || len(s.inFlight) != 0 || len(s.deliveries) != 0 {
			t.Fatalf("completion %d retained state: active=%d claims=%d inFlight=%d deliveries=%d", i, len(s.Filters()), len(s.claims), len(s.inFlight), len(s.deliveries))
		}
		if len(s.finalizer.queue) != 0 || len(s.finalizer.txns) != 0 || len(s.pendingReceipts) != 0 {
			t.Fatalf("completion %d retained queued ownership", i)
		}
	}
}

func TestReceiptsFixCustomValueRemoval(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		for _, exhausted := range []bool{false, true} {
			for _, source := range []string{"original", "Filters", "Receipt.Filter"} {
				t.Run(fmt.Sprintf("pointer=%v/exhausted=%v/%s", pointer, exhausted, source), func(t *testing.T) {
					tx1, _, _ := hardeningTxn(t, 801)
					tx2, _, _ := hardeningTxn(t, 802)
					b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
					l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
					custom := func(label string) Filterer {
						return ownershipValueFilter{FilterLogs(func([]*types.Log) bool { return false }).Finalize(true).ID(7).(Filterer), []string{label}, func(context.Context, Receipt) (bool, error) { return label != "", nil }}
					}
					value := custom("selected").(ownershipValueFilter)
					var q Filterer = value
					if pointer {
						q = &value
					}
					other := custom("surviving")
					s := l.Subscribe(q, other).(*subscriber)
					defer s.Unsubscribe()
					first, second := s.filterers()[0].(*filterOwner), s.filterers()[1].(*filterOwner)
					fromFilters := s.Filters()[0]
					process := func(b *types.Receipt) {
						t.Helper()
						if _, err := s.matchFiltersAndPublish(context.Background(), s.filterers(), []Receipt{{receipt: b}}); err != nil {
							t.Fatal(err)
						}
					}
					process(hardeningReceipt(b1, tx1))
					var mined Receipt
					for i := 0; i < 2; i++ {
						r := hardeningRead(t, s)
						if r.owner == first {
							mined = r
							if pointer {
								if r.Filter != q || fromFilters != q {
									t.Fatal("public pointer identity changed")
								}
							} else if r.Filter.(ownershipValueFilter).Filterer != value.Filterer || fromFilters.(ownershipValueFilter).Filterer != value.Filterer {
								t.Fatal("original public custom value changed")
							}
						}
					}
					if exhausted {
						s.exhaustFilter(first)
						value.Filterer.(*filter).closeExhausted()
					}
					candidate := q
					switch source {
					case "Filters":
						candidate = fromFilters
					case "Receipt.Filter":
						candidate = mined.Filter
					}
					s.RemoveFilter(candidate)
					if s.hasFilter(first) || s.finalizer.hasOwner(first) || !s.hasFilter(second) || !s.finalizer.hasOwner(second) {
						t.Error("public removal did not isolate selected registration/finality")
					}
					process(hardeningReceipt(b2, tx2))
					late := receiptsFixCollect(t, s)
					if len(late) != 1 || late[0].owner != second || late[0].TransactionHash() != tx2.Hash() || late[0].Final || late[0].Reorged {
						t.Error("selected cancellation changed later surviving delivery")
					}
					if err := s.finalizeReceipts(big.NewInt(105)); err != nil {
						t.Fatal(err)
					}
					finals := receiptsFixCollect(t, s)
					seen := make(map[common.Hash]bool)
					for _, r := range finals {
						if r.owner != second || !r.Final || r.Reorged || seen[r.TransactionHash()] {
							t.Error("removed custom value still finalized or surviving final duplicated")
						}
						seen[r.TransactionHash()] = true
					}
					if len(finals) != 2 || !seen[tx1.Hash()] || !seen[tx2.Hash()] {
						t.Error("custom removal lost surviving finals")
					}
				})
			}
		}
	}
}

func TestReceiptsFixSharedBaseAliases(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 803)
	b := hardeningBlock(100, tx)
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	base := FilterLogs(func([]*types.Log) bool { return false }).Finalize(true).(Filterer)
	first := ownershipValueFilter{base, []string{"first"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
	second := ownershipValueFilter{base, []string{"second"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
	other := ownershipValueFilter{FilterLogs(func([]*types.Log) bool { return true }).Finalize(true).(Filterer), []string{"other"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
	s := l.Subscribe(first, second, other).(*subscriber)
	defer s.Unsubscribe()
	owners := s.filterers()
	if _, err := s.matchFiltersAndPublish(context.Background(), owners, []Receipt{{receipt: hardeningReceipt(b, tx)}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		hardeningRead(t, s)
	}
	// Shared base signals define aliases: old RemoveFilter removes the first
	// matching registration. Independent callback identities use distinct bases.
	s.RemoveFilter(second)
	if s.hasFilter(owners[0].(*filterOwner)) || s.finalizer.hasOwner(owners[0].(*filterOwner)) || !s.hasFilter(owners[1].(*filterOwner)) || !s.hasFilter(owners[2].(*filterOwner)) {
		t.Error("shared-base removal did not preserve first-match alias semantics")
	}
	s.RemoveFilter(s.Filters()[0])
	if s.hasFilter(owners[1].(*filterOwner)) || s.finalizer.hasOwner(owners[1].(*filterOwner)) || !s.finalizer.hasOwner(owners[2].(*filterOwner)) {
		t.Error("removing second base alias affected independent owner")
	}
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	finals := receiptsFixCollect(t, s)
	if len(finals) != 1 || finals[0].owner != owners[2] || !finals[0].Final || finals[0].TransactionHash() != tx.Hash() {
		t.Error("shared aliases canceled an unrelated queued final")
	}
}

type receiptsFixComparableFilter struct {
	Filterer
	label string
}

func (f receiptsFixComparableFilter) Match(context.Context, Receipt) (bool, error) {
	return f.label != "", nil
}

func TestReceiptsFixComparableSharedBaseIsolation(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		t.Run(fmt.Sprintf("pointer=%v", pointer), func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 804)
			b := hardeningBlock(100, tx)
			l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
			base := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true).(Filterer)
			var first, second Filterer = receiptsFixComparableFilter{base, "first"}, receiptsFixComparableFilter{base, "second"}
			if pointer {
				first = &ownershipValueFilter{base, []string{"first"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
				second = &ownershipValueFilter{base, []string{"second"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
			}
			s := l.Subscribe(first, second).(*subscriber)
			defer s.Unsubscribe()
			owners := s.filterers()
			if _, err := s.matchFiltersAndPublish(context.Background(), owners, []Receipt{{receipt: hardeningReceipt(b, tx)}}); err != nil {
				t.Fatal(err)
			}
			hardeningRead(t, s)
			hardeningRead(t, s)
			s.RemoveFilter(second)
			if !s.hasFilter(owners[0].(*filterOwner)) || !s.finalizer.hasOwner(owners[0].(*filterOwner)) || s.hasFilter(owners[1].(*filterOwner)) || s.finalizer.hasOwner(owners[1].(*filterOwner)) {
				t.Fatal("base fallback overrode definitive comparable identity")
			}
			if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
				t.Fatal(err)
			}
			finals := receiptsFixCollect(t, s)
			if len(finals) != 1 || finals[0].owner != owners[0] || !finals[0].Final {
				t.Fatal("comparable cancellation lost surviving owner")
			}
		})
	}
}

type receiptsFixNilSignalFilter struct{ ownershipValueFilter }

func (receiptsFixNilSignalFilter) Exhausted() <-chan struct{} { return nil }

func TestReceiptsFixCustomValueWithoutIdentity(t *testing.T) {
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	value := receiptsFixNilSignalFilter{ownershipValueFilter{FilterLogs(func([]*types.Log) bool { return true }).(Filterer), []string{"no identity"}, func(context.Context, Receipt) (bool, error) { return true, nil }}}
	other := FilterLogs(func([]*types.Log) bool { return true }).(Filterer)
	s := l.Subscribe(value, other).(*subscriber)
	defer s.Unsubscribe()
	s.RemoveFilter(s.Filters()[0])
	if len(s.Filters()) != 2 {
		t.Fatal("unidentifiable callback value removed another registration")
	}
	s.ClearFilters()
	s.AddFilter(&value, other)
	s.RemoveFilter(s.Filters()[0])
	if len(s.Filters()) != 1 || s.Filters()[0] != other {
		t.Fatal("pointer identity did not isolate custom nil-signal removal")
	}
}
