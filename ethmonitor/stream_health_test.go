package ethmonitor_test

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// recordingAlerter captures alerts for inspection.
type recordingAlerter struct {
	mu     sync.Mutex
	alerts []string
}

func (a *recordingAlerter) Alert(_ context.Context, format string, v ...interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alerts = append(a.alerts, fmt.Sprintf(format, v...))
}

func (a *recordingAlerter) list() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.alerts...)
}

func newStreamHealthTestMonitor(t *testing.T, chain *fakeChain, alert *recordingAlerter, retryAfter, healthCheckInterval time.Duration) *ethmonitor.Monitor {
	t.Helper()

	provider := &fakeProvider{
		MockRawInterface: mocks.NewMockRawInterface(gomock.NewController(t)),
		chain:            chain,
		streaming:        true,
	}

	opts := ethmonitor.DefaultOptions
	opts.PollingInterval = 20 * time.Millisecond
	opts.Timeout = 2 * time.Second
	opts.StartBlockNumber = new(big.Int).SetUint64(chain.base)
	opts.StreamingErrNumToSwitchToPolling = 2
	opts.StreamingErrorResetInterval = time.Hour
	opts.StreamingRetryAfter = retryAfter
	// header timestamps are second-granular, so the staleness must be above
	// the truncation error; keep it small but > 1s for a fast test.
	opts.StreamHealthCheckInterval = healthCheckInterval
	opts.StreamHealthCheckStaleness = 2 * time.Second
	opts.Alerter = alert

	monitor, err := ethmonitor.NewMonitor(provider, opts)
	require.NoError(t, err)
	return monitor
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, d time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

// TestMonitorStreamHealthCheckFallsBackToPolling stops head notifications
// while the chain keeps producing blocks, reproducing a newHeads stream that
// silently serves nothing while the node's http api stays current. The
// stream health check must drop the subscription, alert, and switch the
// monitor to polling so its head keeps advancing.
func TestMonitorStreamHealthCheckFallsBackToPolling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	chain := newFakeChain(1000, 1, 5*time.Millisecond)
	alerts := &recordingAlerter{}
	monitor := newStreamHealthTestMonitor(t, chain, alerts, time.Hour, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go chain.produce(ctx, 20*time.Millisecond)
	go monitor.Run(ctx)
	defer monitor.Stop()

	waitFor(t, 5*time.Second, "streaming mode with an advancing head", func() bool {
		return monitor.IsStreamingMode() && monitor.LatestBlockNum().Uint64() > chain.base+2
	})
	assert.Empty(t, alerts.list(), "healthy stream must not alert")

	// the stream stalls: no more heads arrive, the chain keeps producing
	chain.mute()
	headAtMute := monitor.LatestBlockNum().Uint64()

	waitFor(t, 10*time.Second, "switch to polling after the stream stalls", func() bool {
		return !monitor.IsStreamingMode()
	})

	// polling keeps the monitor at the head, which is what the node-gateway
	// serves: the incident this guards against left it 23 minutes behind.
	waitFor(t, 5*time.Second, "monitor head to keep advancing in polling mode", func() bool {
		return monitor.LatestBlockNum().Uint64() >= headAtMute+3
	})

	stalled := 0
	for _, a := range alerts.list() {
		if strings.Contains(a, "stalled") {
			stalled++
		}
	}
	assert.Greater(t, stalled, 0, "a stalled stream must alert")
}

// TestMonitorStreamHealthCheckZeroIntervalDisablesCheck runs streaming with
// the health check disabled: heads must keep flowing without a panic, and a
// silent stream must not trip a check that is off.
func TestMonitorStreamHealthCheckZeroIntervalDisablesCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	chain := newFakeChain(1000, 1, 5*time.Millisecond)
	alerts := &recordingAlerter{}
	monitor := newStreamHealthTestMonitor(t, chain, alerts, time.Hour, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go chain.produce(ctx, 20*time.Millisecond)
	go monitor.Run(ctx)
	defer monitor.Stop()

	waitFor(t, 5*time.Second, "streaming mode with an advancing head", func() bool {
		return monitor.IsStreamingMode() && monitor.LatestBlockNum().Uint64() > chain.base+2
	})
	assert.Empty(t, alerts.list(), "the check is off, nothing may alert")

	// the stream goes silent; with the check disabled the monitor must stay
	// in streaming mode rather than trip or panic
	chain.mute()
	time.Sleep(2500 * time.Millisecond)

	assert.True(t, monitor.IsStreamingMode())
	assert.Empty(t, alerts.list())
}

// TestMonitorStreamHealthCheckFreshStreamKeepsStreaming is the control: a
// live stream stays in streaming mode and raises no stall alerts.
func TestMonitorStreamHealthCheckFreshStreamKeepsStreaming(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	chain := newFakeChain(1000, 1, 5*time.Millisecond)
	alerts := &recordingAlerter{}
	monitor := newStreamHealthTestMonitor(t, chain, alerts, time.Hour, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go chain.produce(ctx, 20*time.Millisecond)
	go monitor.Run(ctx)
	defer monitor.Stop()

	waitFor(t, 5*time.Second, "streaming mode with an advancing head", func() bool {
		return monitor.IsStreamingMode() && monitor.LatestBlockNum().Uint64() > chain.base+2
	})

	time.Sleep(1500 * time.Millisecond)

	assert.True(t, monitor.IsStreamingMode())
	for _, a := range alerts.list() {
		assert.NotContains(t, a, "stalled")
	}
}

// TestMonitorStreamHealthCheckRecoversToStreaming stalls the stream until the
// monitor falls back to polling, then resumes heads: the polling listener
// must retry streaming and the monitor must return to streaming mode with an
// advancing head.
func TestMonitorStreamHealthCheckRecoversToStreaming(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	chain := newFakeChain(1000, 1, 5*time.Millisecond)
	alerts := &recordingAlerter{}
	monitor := newStreamHealthTestMonitor(t, chain, alerts, 500*time.Millisecond, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go chain.produce(ctx, 20*time.Millisecond)
	go monitor.Run(ctx)
	defer monitor.Stop()

	waitFor(t, 5*time.Second, "streaming mode with an advancing head", func() bool {
		return monitor.IsStreamingMode() && monitor.LatestBlockNum().Uint64() > chain.base+2
	})

	chain.mute()
	waitFor(t, 15*time.Second, "switch to polling after the stream stalls", func() bool {
		return !monitor.IsStreamingMode()
	})

	chain.unmute()
	waitFor(t, 10*time.Second, "streaming mode with an advancing head after recovery", func() bool {
		return monitor.IsStreamingMode() && monitor.LatestBlockNum().Uint64() > chain.head()-50
	})
}

// TestMonitorStreamHealthCheckStaleHeadDataTrips runs a stream that keeps
// delivering heads, but every head carries a chain timestamp half an hour
// old while the node's http api serves current blocks: the incident shape.
// The check must drop the subscription and fall back to polling even though
// heads are still arriving.
func TestMonitorStreamHealthCheckStaleHeadDataTrips(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	chain := newFakeChain(1000, 1, 5*time.Millisecond)
	alerts := &recordingAlerter{}
	monitor := newStreamHealthTestMonitor(t, chain, alerts, time.Hour, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go chain.produce(ctx, 20*time.Millisecond)
	go monitor.Run(ctx)
	defer monitor.Stop()

	waitFor(t, 5*time.Second, "streaming mode with an advancing head", func() bool {
		return monitor.IsStreamingMode() && monitor.LatestBlockNum().Uint64() > chain.base+2
	})

	chain.serveStaleTimestamps()
	waitFor(t, 10*time.Second, "switch to polling while the stream serves stale data", func() bool {
		return !monitor.IsStreamingMode()
	})

	stalled := 0
	for _, a := range alerts.list() {
		if strings.Contains(a, "stalled") {
			stalled++
		}
	}
	assert.Greater(t, stalled, 0, "a stream serving stale heads must alert")
}
