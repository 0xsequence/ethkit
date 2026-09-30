package ethmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
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
	defer func() { cancel(); <-done }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("prefetch panic did not cancel and join its workers")
	}
}
