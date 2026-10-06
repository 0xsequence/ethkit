package ethmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestPrefetchRejectsInvalidBlockBeforeCaching(t *testing.T) {
	for _, withLogs := range []bool{false, true} {
		t.Run(fmt.Sprintf("withLogs=%v", withLogs), func(t *testing.T) {
			provider := mocks.NewMockRawInterface(gomock.NewController(t))
			provider.EXPECT().RawBlockByNumber(gomock.Any(), big.NewInt(1000)).
				Return(json.RawMessage(`{"number":"0x3e8"}`), nil)
			opts := DefaultOptions
			opts.PrefetchConcurrency = 1
			opts.WithLogs = withLogs
			monitor, err := NewMonitor(provider, opts)
			require.NoError(t, err)
			monitor.chainID = big.NewInt(1)

			monitor.prefetch.fetch(context.Background(), prefetchJob{num: 1000})
			key := CacheKeyBlockByNumber(monitor.chainID, big.NewInt(1000))
			_, found, err := monitor.cache.Get(context.Background(), key)
			require.NoError(t, err)
			require.False(t, found, "worker cached an undecodable block")
		})
	}
}

func TestMonitorRecoversInvalidPrefetchedLogs(t *testing.T) {
	for _, payload := range []string{`{}`, `[{}]`} {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("payload=%s/cached=%v", payload, cached), func(t *testing.T) {
				const first, target, last = uint64(1000), uint64(1002), uint64(1004)
				targetHash := common.BigToHash(new(big.Int).SetUint64(target))
				expectedLogs := []types.Log{{
					Address:     common.HexToAddress("0x1234"),
					Topics:      []common.Hash{common.HexToHash("0xabcd")},
					Data:        []byte{1, 2},
					TxHash:      common.HexToHash("0x5678"),
					BlockHash:   targetHash,
					BlockNumber: target,
				}}
				validPayload, err := json.Marshal(expectedLogs)
				require.NoError(t, err)

				provider := mocks.NewMockRawInterface(gomock.NewController(t))
				provider.EXPECT().ChainID(gomock.Any()).Return(big.NewInt(1), nil).AnyTimes()
				provider.EXPECT().IsStreamingEnabled().Return(false).AnyTimes()
				provider.EXPECT().BlockNumber(gomock.Any()).Return(last, nil).AnyTimes()
				provider.EXPECT().RawBlockByNumber(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, number *big.Int) (json.RawMessage, error) {
						if err := ctx.Err(); err != nil {
							return nil, err
						}
						if number.Uint64() > last {
							return nil, ethereum.NotFound
						}
						var bloom types.Bloom
						if number.Uint64() == target {
							bloom = types.BytesToBloom([]byte{1})
						}
						header := &types.Header{
							Number:     number,
							ParentHash: common.BigToHash(new(big.Int).Sub(number, big.NewInt(1))),
							Difficulty: big.NewInt(0),
							GasLimit:   30_000_000,
							Bloom:      bloom,
						}
						header.SetHash(common.BigToHash(number))
						return json.Marshal(header)
					},
				).AnyTimes()
				var originCalls atomic.Int64
				provider.EXPECT().RawFilterLogs(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, query ethereum.FilterQuery) (json.RawMessage, error) {
						if err := ctx.Err(); err != nil {
							return nil, err
						}
						if *query.BlockHash != targetHash {
							return json.RawMessage(`[]`), nil
						}
						if originCalls.Add(1) == 1 && !cached {
							return json.RawMessage(payload), nil
						}
						return json.RawMessage(validPayload), nil
					},
				).AnyTimes()

				opts := DefaultOptions
				opts.WithLogs = true
				opts.PrefetchConcurrency = 1
				opts.StartBlockNumber = new(big.Int).SetUint64(first)
				opts.PollingInterval = 10 * time.Millisecond
				opts.CacheExpiry = time.Hour
				// Leave CacheBackend unset to exercise automatic memory caching.
				monitor, err := NewMonitor(provider, opts)
				require.NoError(t, err)
				monitor.chainID = big.NewInt(1)
				key := CacheKeyBlockLogs(monitor.chainID, targetHash, monitor.logTopics())
				if cached {
					require.NoError(t, monitor.cache.SetEx(context.Background(), key, []byte(payload), time.Hour))
				} else {
					// Complete the speculative fetch before starting the serial loop,
					// so the worker deterministically receives the malformed response.
					monitor.prefetch.fetch(context.Background(), prefetchJob{num: target})
					require.Equal(t, int64(1), originCalls.Load())
					_, found, err := monitor.cache.Get(context.Background(), key)
					require.NoError(t, err)
					require.False(t, found, "failed prefetch must not cache malformed logs")
				}

				sub := monitor.Subscribe("TestMonitorRecoversInvalidPrefetchedLogs")
				defer sub.Unsubscribe()
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan error, 1)
				go func() { done <- monitor.Run(ctx) }()
				defer func() {
					cancel()
					select {
					case err := <-done:
						require.NoError(t, err)
					case <-time.After(5 * time.Second):
						t.Error("monitor did not stop")
					}
				}()

				timer := time.NewTimer(3 * time.Second)
				defer timer.Stop()
				next := first
				for next <= last {
					select {
					case blocks := <-sub.Blocks():
						for _, block := range blocks {
							require.Equal(t, Added, block.Event)
							require.True(t, block.OK)
							require.Equal(t, next, block.NumberU64(), "blocks must publish in order")
							if next == target {
								require.Equal(t, expectedLogs, block.Logs)
							}
							next++
						}
					case <-timer.C:
						t.Fatalf("publication stalled at block %d; origin calls: %d", next, originCalls.Load())
					}
				}
				if !cached {
					require.GreaterOrEqual(t, originCalls.Load(), int64(2), "malformed prefetch must allow an origin retry")
				} else {
					require.Positive(t, originCalls.Load(), "invalid cache entry must allow an origin retry")
				}
				stored, found, err := monitor.cache.Get(context.Background(), key)
				require.NoError(t, err)
				require.True(t, found)
				require.JSONEq(t, string(validPayload), string(stored))
			})
		}
	}
}

func TestPrefetchPanicStopsWorkers(t *testing.T) {
	provider := mocks.NewMockRawInterface(gomock.NewController(t))
	provider.EXPECT().BlockNumber(gomock.Any()).DoAndReturn(func(context.Context) (uint64, error) {
		panic("simulated head poll panic")
	})
	opts := DefaultOptions
	opts.PrefetchConcurrency = 4
	monitor, err := NewMonitor(provider, opts)
	require.NoError(t, err)
	monitor.hitStreak.Store(2)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitor.prefetch.run(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("prefetch did not stop after cancellation")
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("prefetch panic did not cancel and join its workers")
	}
}
