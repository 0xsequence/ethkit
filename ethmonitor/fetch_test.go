package ethmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	memcache "github.com/goware/cachestore-mem"
	cachestore "github.com/goware/cachestore2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func fetchTestPayload(t *testing.T) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(&types.Header{
		Number: big.NewInt(1000), Difficulty: big.NewInt(0), GasLimit: 30_000_000,
	})
	require.NoError(t, err)
	return payload
}

func TestMonitorFetchNextBlockCacheTimeout(t *testing.T) {
	for _, concurrency := range []int{0, 1} {
		t.Run(fmt.Sprintf("prefetch=%d", concurrency), func(t *testing.T) {
			// Use the actual memory backend with a shorter timeout than its
			// eight-second default. Waiting for a slow head must outlive it.
			backend, err := memcache.NewBackend(512, cachestore.WithLockRetryTimeout(25*time.Millisecond))
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			callerDeadline, _ := ctx.Deadline()
			var readyAt time.Time
			payload := fetchTestPayload(t)
			usedCallerDeadline := false
			provider := mocks.NewMockRawInterface(gomock.NewController(t))
			provider.EXPECT().RawBlockByNumber(gomock.Any(), big.NewInt(1000)).DoAndReturn(
				func(ctx context.Context, _ *big.Int) (json.RawMessage, error) {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					if readyAt.IsZero() {
						readyAt = time.Now().Add(100 * time.Millisecond)
					}
					deadline, _ := ctx.Deadline()
					usedCallerDeadline = usedCallerDeadline || deadline.Equal(callerDeadline)
					if time.Now().Before(readyAt) {
						return nil, ethereum.NotFound
					}
					return payload, nil
				},
			).AnyTimes()
			opts := DefaultOptions
			opts.CacheBackend = backend
			opts.PrefetchConcurrency = concurrency
			opts.PollingInterval = time.Millisecond
			monitor, err := NewMonitor(provider, opts)
			require.NoError(t, err)
			monitor.chainID, monitor.nextBlockNumber = big.NewInt(1), big.NewInt(1000)

			block, response, miss, err := monitor.fetchNextBlock(ctx, false)
			require.NoError(t, err)
			require.Equal(t, uint64(1000), block.NumberU64())
			require.Equal(t, []byte(payload), response)
			require.True(t, miss)
			require.True(t, usedCallerDeadline, "origin retry inherited the cache's shorter deadline")
		})
	}
}

// Memory-cache singleflight can return another caller's getter error without
// invoking this caller's getter. Inject that failure without relying on timing.
type fetchErrorCache struct {
	cachestore.Store[[]byte]
	err    error
	cancel context.CancelFunc
}

func (c *fetchErrorCache) GetOrSetWithLockEx(context.Context, string, func(context.Context, string) ([]byte, error), time.Duration) ([]byte, error) {
	if c.cancel != nil {
		c.cancel()
	}
	return nil, fmt.Errorf("cache getter: %w", c.err)
}

func TestMonitorFetchNextBlockCacheFailure(t *testing.T) {
	for _, tc := range []struct {
		name          string
		cacheErr      error
		cancel        bool
		invalidOrigin bool
	}{
		{name: "shared_prefetch_not_found", cacheErr: ethereum.NotFound},
		{name: "shared_prefetch_timeout", cacheErr: context.DeadlineExceeded},
		{name: "backend_failure", cacheErr: errors.New("cache unavailable")},
		{name: "caller_canceled", cacheErr: ethereum.NotFound, cancel: true},
		{name: "invalid_origin", cacheErr: ethereum.NotFound, invalidOrigin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			provider := mocks.NewMockRawInterface(gomock.NewController(t))
			payload := fetchTestPayload(t)
			if tc.invalidOrigin {
				payload = json.RawMessage(`{"number":"0x3e8"}`)
			}
			if !tc.cancel {
				provider.EXPECT().RawBlockByNumber(gomock.Any(), big.NewInt(1000)).Return(payload, nil)
			}
			opts := DefaultOptions
			opts.PrefetchConcurrency = 1
			monitor, err := NewMonitor(provider, opts)
			require.NoError(t, err)
			monitor.chainID, monitor.nextBlockNumber = big.NewInt(1), big.NewInt(1000)
			cache := &fetchErrorCache{Store: monitor.cache, err: tc.cacheErr}
			if tc.cancel {
				cache.cancel = cancel
			}
			monitor.cache = cache
			monitor.hitStreak.Store(2)

			block, response, miss, err := monitor.fetchNextBlock(ctx, false)
			switch {
			case tc.cancel:
				require.ErrorIs(t, err, context.Canceled)
				require.Nil(t, block) // The mock also rejects any origin call.
			case tc.invalidOrigin:
				require.Error(t, err)
				require.NotErrorIs(t, err, tc.cacheErr, "origin validation was never attempted")
				require.Nil(t, block)
			default:
				require.NoError(t, err)
				require.Equal(t, uint64(1000), block.NumberU64())
				require.Equal(t, []byte(payload), response)
				require.True(t, miss)
				require.Zero(t, monitor.hitStreak.Load(), "cache failure left the catch-up signal active")
			}
		})
	}
}
