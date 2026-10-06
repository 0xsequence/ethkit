package ethmonitor_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	memcache "github.com/goware/cachestore-mem"
	cachestore "github.com/goware/cachestore2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Fail before invoking the getter, as an unreachable shared cache would.
type outageBackend struct {
	cachestore.Backend
	down     atomic.Bool
	failures atomic.Int64
}

func (b *outageBackend) GetOrSetWithLockEx(ctx context.Context, key string, getter func(context.Context, string) (any, error), ttl time.Duration) (any, error) {
	if b.down.Load() {
		b.failures.Add(1)
		return nil, errors.New("cache unavailable")
	}
	return b.Backend.GetOrSetWithLockEx(ctx, key, getter, ttl)
}

// Give each fake block a nonzero bloom and an actual log to verify that
// outage recovery preserves logs, rather than just marking blocks ready.
type cacheTestProvider struct {
	*fakeProvider
	log types.Log
}

func (p *cacheTestProvider) RawBlockByNumber(ctx context.Context, num *big.Int) (json.RawMessage, error) {
	payload, err := p.fakeProvider.RawBlockByNumber(ctx, num)
	if err != nil {
		return nil, err
	}
	bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{&p.log}})
	return json.RawMessage(strings.Replace(string(payload), strings.Repeat("0", 512), common.Bytes2Hex(bloom[:]), 1)), nil
}

func (p *cacheTestProvider) RawFilterLogs(ctx context.Context, q ethereum.FilterQuery) (json.RawMessage, error) {
	if err := p.chain.wait(ctx); err != nil {
		return nil, err
	}
	block, ok := p.chain.byHash(*q.BlockHash)
	if !ok {
		return nil, ethereum.NotFound
	}
	log := p.log
	log.BlockHash, log.BlockNumber = block.hash, block.num
	return json.Marshal([]types.Log{log})
}

func TestMonitorCacheOutageWithLogs(t *testing.T) {
	for _, concurrency := range []int{0, 2} {
		t.Run(fmt.Sprintf("prefetch=%d", concurrency), func(t *testing.T) {
			chain := newFakeChain(1000, 40, 0)
			chain.hold(1020)
			provider := &cacheTestProvider{
				fakeProvider: &fakeProvider{MockRawInterface: mocks.NewMockRawInterface(gomock.NewController(t)), chain: chain},
				log:          types.Log{Address: common.HexToAddress("0x1234"), Topics: []common.Hash{common.HexToHash("0xabcd")}, Data: []byte{1, 2, 3}},
			}
			mem, err := memcache.NewBackend(512)
			require.NoError(t, err)
			backend := &outageBackend{Backend: mem}
			backend.down.Store(true)
			opts := ethmonitor.DefaultOptions
			opts.PollingInterval = 5 * time.Millisecond
			opts.Timeout = time.Second
			opts.WithLogs = true
			opts.BlockRetentionLimit = 5 // Queue capacity is 10; publish 20 blocks while down.
			opts.StartBlockNumber = big.NewInt(1000)
			opts.PrefetchConcurrency = concurrency
			opts.CacheBackend = backend
			monitor, err := ethmonitor.NewMonitor(provider, opts)
			require.NoError(t, err)
			sub := monitor.Subscribe()
			defer sub.Unsubscribe()
			done := runMonitorForTest(t, monitor)
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			next := uint64(1000)
			for next <= 1039 {
				select {
				case blocks := <-sub.Blocks():
					for _, block := range blocks {
						require.Equal(t, next, block.NumberU64())
						require.Equal(t, ethmonitor.Added, block.Event)
						require.True(t, block.OK)
						require.NotZero(t, block.Bloom())
						log := provider.log
						log.BlockHash, log.BlockNumber = block.Hash(), next
						require.Equal(t, []types.Log{log}, block.Logs)
						next++
						if next == 1020 {
							require.True(t, backend.down.Load())
							require.GreaterOrEqual(t, backend.failures.Load(), int64(40))
							backend.down.Store(false)
							chain.release(1020)
						}
					}
				case err := <-done:
					t.Fatalf("Run returned during cache outage/recovery: %v (head=%v)", err, monitor.LatestBlockNum())
				case <-timer.C:
					t.Fatalf("monitor stopped publishing at block %d", next-1)
				}
			}
			// After recovery, ordinary reads fill the cache again without a restart.
			key := ethmonitor.CacheKeyBlockLogs(big.NewInt(1), chain.hashAt(1039), nil)
			_, found, err := backend.Get(context.Background(), key)
			require.NoError(t, err)
			require.True(t, found)
			require.True(t, monitor.IsRunning())
		})
	}
}
