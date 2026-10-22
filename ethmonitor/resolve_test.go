package ethmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLogsAttemptsFallBackToReceipts(t *testing.T) {
	topicA, topicB := common.HexToHash("0xaa"), common.HexToHash("0xbb")
	logsDown := &jsonrpc.Error{Code: -32000, Message: "logs down"}

	cases := []struct {
		name      string
		settled   bool
		txs       int
		bloom     types.Bloom
		topics    []common.Hash
		behavior  receiptsBehavior
		receipts  func(block *types.Block) []types.Log
		calls     int
		wantReady []bool // after each call
		wantLogs  func(receipts []types.Log) []types.Log

		wantBlockReceipts int // eth_getBlockReceipts calls in total, checked when non-zero
	}{
		{
			name: "a_empty_logs_with_nonzero_bloom", txs: 1, bloom: nonzeroBloom, calls: 3,
			receipts: func(b *types.Block) []types.Log {
				return []types.Log{newTestLog(b, 0, topicA), newTestLog(b, 1, topicB)}
			},
			wantReady: []bool{false, false, true},
			wantLogs:  func(r []types.Log) []types.Log { return r },
		},
		{
			name: "b_logs_error_receipts_empty", txs: 2, behavior: receiptsBehavior{logsErr: logsDown}, calls: 3,
			receipts:  func(b *types.Block) []types.Log { return nil },
			wantReady: []bool{false, false, true},
			wantLogs:  func(r []types.Log) []types.Log { return []types.Log{} },
		},
		{
			name: "c_topic_filtered_receipts", txs: 1, topics: []common.Hash{topicA}, behavior: receiptsBehavior{logsErr: logsDown}, calls: 3,
			receipts: func(b *types.Block) []types.Log {
				return []types.Log{newTestLog(b, 0, topicA), newTestLog(b, 1, topicB)}
			},
			wantReady: []bool{false, false, true},
			wantLogs:  func(r []types.Log) []types.Log { return r[:1] },
		},
		{
			// a transient JSON-RPC error doesn't disable eth_getBlockReceipts:
			// it's asked again on each of attempts 3, 4 and 5
			name: "d_receipts_fail_too", txs: 1, behavior: receiptsBehavior{logsErr: logsDown, receiptsErr: errReceiptsDown}, calls: 5,
			receipts:          func(b *types.Block) []types.Log { return nil },
			wantReady:         []bool{false, false, false, false, false},
			wantBlockReceipts: 3,
		},
		{
			// receipts without logs contradict a non-zero bloom outside
			// settled-header chains, so they aren't trusted either
			name: "e_empty_receipts_contradict_nonzero_bloom", txs: 1, bloom: nonzeroBloom, calls: 5,
			receipts:          func(b *types.Block) []types.Log { return nil },
			wantReady:         []bool{false, false, false, false, false},
			wantBlockReceipts: 3,
		},
		{
			// on settled-header chains filterLogs already asks for receipts on
			// every attempt, so addLogs doesn't ask a second time once the
			// attempts are used up: one receipts call per attempt
			name: "f_settled_receipts_fail_one_call_per_attempt", settled: true, txs: 1, behavior: receiptsBehavior{receiptsErr: errReceiptsDown}, calls: 5,
			receipts:          func(b *types.Block) []types.Log { return nil },
			wantReady:         []bool{false, false, false, false, false},
			wantBlockReceipts: 5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := newTxBlock(t, 1000, tc.txs, tc.bloom)
			receiptLogs := tc.receipts(block)
			behavior := tc.behavior
			behavior.receiptLogs = map[common.Hash][]types.Log{block.Hash(): receiptLogs}
			node := newReceiptsNode(t, []*types.Block{block}, &behavior)
			provider, err := ethrpc.NewProvider(node.URL)
			require.NoError(t, err)

			opts := DefaultOptions
			opts.WithLogs = true
			opts.MaxLogsAttempts = 3
			opts.LogTopics = tc.topics
			monitor, err := NewMonitor(provider, opts)
			require.NoError(t, err)
			monitor.chainID = big.NewInt(1)
			monitor.settledHeaders.Store(tc.settled)

			b := &Block{Block: block, Event: Added}
			for i := 0; i < tc.calls; i++ {
				monitor.addLogs(context.Background(), Blocks{b})
				require.Equal(t, tc.wantReady[i], b.OK, "call %d", i+1)
				if i < 2 && !tc.settled {
					// receipts are only consulted once the attempts are used up
					require.Zero(t, node.calls("eth_getBlockReceipts"), "call %d", i+1)
				}
			}
			if tc.wantBlockReceipts > 0 {
				require.Equal(t, tc.wantBlockReceipts, node.calls("eth_getBlockReceipts"))
				require.Zero(t, node.calls("eth_getTransactionReceipt"), "eth_getBlockReceipts must not be given up on")
			}
			if tc.wantLogs == nil {
				require.Nil(t, b.Logs)
				return
			}
			want := tc.wantLogs(receiptLogs)
			require.NotNil(t, b.Logs)
			if len(want) == 0 {
				require.Empty(t, b.Logs)
			} else {
				require.Equal(t, want, b.Logs)
			}
		})
	}
}

// recordingHandler is a slog.Handler which keeps every record.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

// messages returns the messages at level which contain substr.
func (h *recordingHandler) messages(level slog.Level, substr string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, r := range h.records {
		if r.Level == level && strings.Contains(r.Message, substr) {
			out = append(out, r.Message)
		}
	}
	return out
}

// recordingAlerter is a util.Alerter which keeps every alert.
type recordingAlerter struct {
	mu     sync.Mutex
	alerts []string
}

func (a *recordingAlerter) Alert(_ context.Context, format string, v ...interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alerts = append(a.alerts, fmt.Sprintf(format, v...))
}

func (a *recordingAlerter) messages() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.alerts...)
}

// testBlockPayload is the JSON a node returns for a block built by newTxBlock,
// optionally with a settledHeight field.
func testBlockPayload(t *testing.T, number uint64, txCount int, bloom types.Bloom, settled bool) json.RawMessage {
	t.Helper()
	block := newTxBlock(t, number, txCount, bloom)
	headerJSON, err := json.Marshal(block.Header())
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(headerJSON, &payload))

	txs := make([]json.RawMessage, 0, txCount)
	for _, tx := range block.Transactions() {
		txJSON, err := json.Marshal(tx)
		require.NoError(t, err)
		txs = append(txs, txJSON)
	}
	payload["transactions"], err = json.Marshal(txs)
	require.NoError(t, err)
	payload["uncles"] = json.RawMessage(`[]`)
	if settled {
		payload["settledHeight"] = json.RawMessage(fmt.Sprintf(`"0x%x"`, number-1))
	}
	out, err := json.Marshal(payload)
	require.NoError(t, err)
	return out
}

// newMockChainProvider serves blocks first..last, built by testBlockPayload.
// Their hashes are those of newTxBlock. eth_getLogs answers with logs and
// every receipts call with receipts, as given.
func newMockChainProvider(t *testing.T, first, last uint64, txCount int, bloom types.Bloom, settled bool,
	logs func() (json.RawMessage, error), receipts func() error) *mocks.MockRawInterface {
	t.Helper()

	payloads := map[uint64]json.RawMessage{}
	hashes := map[common.Hash]uint64{}
	for n := first; n <= last; n++ {
		payloads[n] = testBlockPayload(t, n, txCount, bloom, settled)
		hashes[newTxBlock(t, n, 0, bloom).Hash()] = n
	}

	provider := mocks.NewMockRawInterface(gomock.NewController(t))
	provider.EXPECT().ChainID(gomock.Any()).Return(big.NewInt(1), nil).AnyTimes()
	provider.EXPECT().IsStreamingEnabled().Return(false).AnyTimes()
	provider.EXPECT().BlockNumber(gomock.Any()).Return(last, nil).AnyTimes()
	provider.EXPECT().RawBlockByNumber(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, number *big.Int) (json.RawMessage, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			n := last
			if number != nil {
				n = number.Uint64()
			}
			payload, ok := payloads[n]
			if !ok {
				return nil, ethereum.NotFound
			}
			return payload, nil
		}).AnyTimes()
	provider.EXPECT().RawBlockByHash(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, hash common.Hash) (json.RawMessage, error) {
			payload, ok := payloads[hashes[hash]]
			if !ok {
				return nil, ethereum.NotFound
			}
			return payload, nil
		}).AnyTimes()
	provider.EXPECT().RawFilterLogs(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ ethereum.FilterQuery) (json.RawMessage, error) {
			return logs()
		}).AnyTimes()
	provider.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ ...ethrpc.Call) ([]byte, error) {
			return nil, receipts()
		}).AnyTimes()
	return provider
}

// runTestMonitor runs the monitor until the test ends. The returned channel
// yields Run's error, and is closed after.
func runTestMonitor(t *testing.T, monitor *Monitor) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- monitor.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("monitor did not shut down")
		}
	})
	return done
}

// collectBlocks records every block the subscription receives.
type collectedBlocks struct {
	mu     sync.Mutex
	blocks []*Block
}

func collectSubscription(t *testing.T, monitor *Monitor) *collectedBlocks {
	t.Helper()
	sub := monitor.Subscribe(t.Name())
	stop := make(chan struct{})
	collected := &collectedBlocks{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case blocks := <-sub.Blocks():
				collected.mu.Lock()
				collected.blocks = append(collected.blocks, blocks...)
				collected.mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		wg.Wait()
		sub.Unsubscribe()
	})
	return collected
}

func (c *collectedBlocks) snapshot() []*Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*Block{}, c.blocks...)
}

func TestMonitorUnresolvedBlocksAreNotFatal(t *testing.T) {
	// Blocks carry transactions and both eth_getLogs and every receipts call
	// fail, so no block can be resolved. The chain is served by a mocked
	// provider, as the fake chain's blocks have no transactions.
	const (
		first     = uint64(1000)
		retention = 10
		numBlocks = 3*retention + 5
		last      = first + numBlocks - 1
	)
	newMonitor := func(t *testing.T, strict bool) (*Monitor, *recordingHandler, *recordingAlerter) {
		provider := newMockChainProvider(t, first, last, 1, types.Bloom{}, false,
			func() (json.RawMessage, error) { return nil, errors.New("logs down") },
			func() error { return errors.New("receipts down") })
		handler, alerter := &recordingHandler{}, &recordingAlerter{}
		opts := DefaultOptions
		opts.Logger = slog.New(handler)
		opts.Alerter = alerter
		opts.WithLogs = true
		opts.StrictLogs = strict
		opts.BlockRetentionLimit = retention
		opts.MaxLogsAttempts = 3
		opts.StartBlockNumber = new(big.Int).SetUint64(first)
		opts.PollingInterval = time.Millisecond
		opts.Timeout = 2 * time.Second
		monitor, err := NewMonitor(provider, opts)
		require.NoError(t, err)
		return monitor, handler, alerter
	}

	t.Run("default", func(t *testing.T) {
		monitor, handler, alerter := newMonitor(t, false)
		collected := collectSubscription(t, monitor)
		done := runTestMonitor(t, monitor)

		// A block is published without logs once it has left the chain cache,
		// which holds the last `retention` blocks.
		published := int(numBlocks - retention)
		var runErr atomic.Value
		require.Eventually(t, func() bool {
			select {
			case err := <-done:
				runErr.Store(fmt.Sprintf("%v", err))
				return true
			default:
			}
			return len(collected.snapshot()) >= published
		}, 10*time.Second, 5*time.Millisecond)
		require.Nil(t, runErr.Load(), "Run returned")
		select {
		case err := <-done:
			t.Fatalf("Run returned: %v", err)
		case <-time.After(100 * time.Millisecond):
		}

		blocks := collected.snapshot()
		require.Len(t, blocks, published)
		require.Greater(t, len(blocks), 2*retention, "more blocks than the publish queue holds")
		for i, block := range blocks {
			require.Equal(t, first+uint64(i), block.NumberU64())
			require.Equal(t, Added, block.Event)
			require.True(t, block.OK)
			require.True(t, block.LogsUnconfirmed)
			require.NotNil(t, block.Logs)
			require.Empty(t, block.Logs)
		}

		errorLogs := handler.messages(slog.LevelError, "publishing without logs")
		require.Len(t, errorLogs, published)
		alerts := alerter.messages()
		require.Len(t, alerts, published)
		for _, block := range blocks {
			hash := block.Hash().Hex()
			require.Len(t, filterContaining(errorLogs, hash), 1, "Error logs for block %d", block.NumberU64())
			require.Len(t, filterContaining(alerts, hash), 1, "alerts for block %d", block.NumberU64())
		}
	})

	t.Run("strict", func(t *testing.T) {
		monitor, _, alerter := newMonitor(t, true)
		collected := collectSubscription(t, monitor)
		done := runTestMonitor(t, monitor)

		select {
		case err := <-done:
			require.ErrorIs(t, err, ErrFatal)
			require.ErrorIs(t, err, ErrQueueFull)
		case <-time.After(10 * time.Second):
			t.Fatal("expected the publish queue to overflow")
		}
		require.Empty(t, collected.snapshot())
		require.Empty(t, alerter.messages())
	})
}

func filterContaining(messages []string, substr string) []string {
	var out []string
	for _, m := range messages {
		if strings.Contains(m, substr) {
			out = append(out, m)
		}
	}
	return out
}

func TestBlockLogsUnconfirmedSurvivesCopyAndSnapshot(t *testing.T) {
	for _, unconfirmed := range []bool{false, true} {
		t.Run(fmt.Sprintf("unconfirmed=%v", unconfirmed), func(t *testing.T) {
			block := canonicalTestBlock(100)
			block.OK = true
			block.Logs = []types.Log{}
			block.LogsUnconfirmed = unconfirmed

			require.Equal(t, unconfirmed, Blocks{block}.Copy()[0].LogsUnconfirmed)

			data, err := json.Marshal(block)
			require.NoError(t, err)
			var decoded Block
			require.NoError(t, json.Unmarshal(data, &decoded))
			require.Equal(t, unconfirmed, decoded.LogsUnconfirmed)
		})
	}
}
