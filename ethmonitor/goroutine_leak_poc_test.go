package ethmonitor_test

//go:generate mockgen -destination=internal/mocks/mock_provider.go -package=mocks github.com/0xsequence/ethkit/ethrpc RawInterface

import (
	"context"
	"fmt"
	"math/big"
	"runtime"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
	"go.uber.org/mock/gomock"
)

// TestMonitorShutdownGoroutineLeak demonstrates a goroutine leak on shutdown.
func TestMonitorShutdownGoroutineLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := mocks.NewMockRawInterface(ctrl)
	provider.EXPECT().ChainID(gomock.Any()).Return(big.NewInt(1), nil).AnyTimes()
	provider.EXPECT().IsStreamingEnabled().Return(false).AnyTimes()
	provider.EXPECT().RawBlockByNumber(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("simulated network error")).AnyTimes()

	baseline := runtime.NumGoroutine()

	opts := ethmonitor.DefaultOptions
	opts.PollingInterval = 10 * time.Millisecond
	opts.Timeout = 50 * time.Millisecond

	monitor, err := ethmonitor.NewMonitor(provider, opts)
	if err != nil {
		t.Fatalf("failed to create monitor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- monitor.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Monitor.Run() didn't return within timeout")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	if leaked := runtime.NumGoroutine() - baseline; leaked > 0 {
		t.Fatalf("%d goroutine(s) leaked", leaked)
	}
}
