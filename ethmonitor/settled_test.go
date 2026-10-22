package ethmonitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/goware/superr"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHasSettledHeight(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		{name: "hex", payload: `{"number":"0x1","settledHeight":"0x5"}`, want: true},
		{name: "number", payload: `{"number":"0x1","settledHeight":5}`, want: true},
		{name: "zero", payload: `{"number":"0x1","settledHeight":"0x0"}`, want: true},
		{name: "spaced", payload: `{"number":"0x1", "settledHeight" : "0x5"}`, want: true},
		{name: "absent", payload: `{"number":"0x1","hash":"0x2"}`},
		{name: "null", payload: `{"number":"0x1","settledHeight":null}`},
		{name: "spaced_null", payload: `{"number":"0x1","settledHeight": null }`},
		{name: "malformed_json", payload: `{"number":"0x1","settledHeight":"0x5"`},
		{name: "not_json", payload: `"settledHeight"`},
		{name: "empty", payload: ``},
		// the key inside a string, which the byte pre-check alone can't rule out
		{name: "key_in_transaction_input", payload: `{"number":"0x1","transactions":[{"input":"{\"settledHeight\":\"0x5\"}"}]}`},
		{name: "key_in_string_value", payload: `{"number":"0x1","extraData":"\"settledHeight\""}`},
		// the pre-check passes and the decoder must not accept a nested key
		{name: "nested_key_only", payload: `{"number":"0x1","transactions":[{"settledHeight":"0x5"}]}`},
		{name: "nested_object_only", payload: `{"number":"0x1","extra":{"settledHeight":"0x5"}}`},
		{name: "top_level_and_nested", payload: `{"extra":{"settledHeight":"0x5"},"settledHeight":"0x6"}`, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, hasSettledHeight([]byte(tc.payload)))
		})
	}
}

func TestSettledHeadersDetection(t *testing.T) {
	const detected = "settled block headers detected"

	newMonitor := func(t *testing.T) (*Monitor, *recordingHandler) {
		handler := &recordingHandler{}
		opts := DefaultOptions
		opts.Logger = slog.New(handler)
		monitor, err := NewMonitor(mocks.NewMockRawInterface(gomock.NewController(t)), opts)
		require.NoError(t, err)
		return monitor, handler
	}

	t.Run("settledHeight_flips_flag_and_logs_once", func(t *testing.T) {
		monitor, handler := newMonitor(t)
		require.False(t, monitor.usesSettledHeaders())

		for n := uint64(1000); n < 1002; n++ {
			block, err := monitor.unmarshalBlock(testBlockPayload(t, n, 0, nonzeroBloom, true))
			require.NoError(t, err)
			require.Equal(t, n, block.NumberU64())
			require.True(t, monitor.usesSettledHeaders())
		}
		require.Len(t, handler.messages(slog.LevelInfo, detected), 1)
	})

	t.Run("no_settledHeight_leaves_flag_false", func(t *testing.T) {
		monitor, handler := newMonitor(t)
		for n := uint64(1000); n < 1002; n++ {
			_, err := monitor.unmarshalBlock(testBlockPayload(t, n, 0, nonzeroBloom, false))
			require.NoError(t, err)
			require.False(t, monitor.usesSettledHeaders())
		}
		require.Empty(t, handler.messages(slog.LevelInfo, detected))
	})

	t.Run("invalid_payload_does_not_flip_flag", func(t *testing.T) {
		monitor, _ := newMonitor(t)
		_, err := monitor.unmarshalBlock([]byte(`{"settledHeight":"0x5"`))
		require.Error(t, err)
		require.False(t, monitor.usesSettledHeaders())
	})
}

func TestMonitorSettledChainDoesNotStall(t *testing.T) {
	// Every block has a non-zero bloom and eth_getLogs answers []. On a
	// settled-header chain that is a premature answer for a block which
	// has no logs of its own, and must not stall the monitor.
	const (
		first = uint64(1000)
		count = 25
		last  = first + count - 1
	)
	newMonitor := func(t *testing.T, settled bool, mutate func(*Options)) (*Monitor, *atomic.Int64) {
		var receiptCalls atomic.Int64
		provider := newMockChainProvider(t, first, last, 0, nonzeroBloom, settled,
			func() (json.RawMessage, error) { return json.RawMessage(`[]`), nil },
			func() error {
				receiptCalls.Add(1)
				return errors.New("receipts down")
			})
		opts := DefaultOptions
		opts.WithLogs = true
		opts.StartBlockNumber = new(big.Int).SetUint64(first)
		opts.PollingInterval = time.Millisecond
		opts.Timeout = 2 * time.Second
		mutate(&opts)
		monitor, err := NewMonitor(provider, opts)
		require.NoError(t, err)
		return monitor, &receiptCalls
	}

	t.Run("settled", func(t *testing.T) {
		monitor, receiptCalls := newMonitor(t, true, func(o *Options) {
			o.BlockRetentionLimit = 6 // the publish queue holds 12, fewer than the 25 blocks
		})
		collected := collectSubscription(t, monitor)
		done := runTestMonitor(t, monitor)

		var runErr atomic.Value
		require.Eventually(t, func() bool {
			select {
			case err := <-done:
				runErr.Store(err)
				return true
			default:
			}
			return len(collected.snapshot()) >= count
		}, 10*time.Second, 5*time.Millisecond)
		require.Nil(t, runErr.Load(), "Run returned")

		blocks := collected.snapshot()
		require.Len(t, blocks, count)
		for i, block := range blocks {
			require.Equal(t, first+uint64(i), block.NumberU64())
			require.Equal(t, Added, block.Event)
			require.True(t, block.OK)
			require.False(t, block.LogsUnconfirmed)
			require.Empty(t, block.Logs)
		}
		require.Zero(t, receiptCalls.Load(), "blocks without transactions need no receipts")
		require.True(t, monitor.IsRunning())
	})

	// Without settledHeight the non-zero bloom makes the empty logs invalid, as
	// before. StrictLogs and a high MaxLogsAttempts turn off the receipts
	// fallback, which would otherwise resolve these transaction-less blocks.
	// The queue holds 4 events, fewer than the 10 blocks the chain retains, so
	// it overflows before any block is evicted.
	t.Run("control_not_settled_stalls", func(t *testing.T) {
		monitor, receiptCalls := newMonitor(t, false, func(o *Options) {
			o.BlockRetentionLimit = 2
			o.StrictLogs = true
			o.MaxLogsAttempts = 1000
		})
		collected := collectSubscription(t, monitor)
		done := runTestMonitor(t, monitor)

		select {
		case err := <-done:
			require.ErrorIs(t, err, ErrFatal)
			require.ErrorIs(t, err, ErrQueueFull)
		case <-time.After(10 * time.Second):
			t.Fatal("expected the publish queue to overflow")
		}
		require.Empty(t, collected.snapshot(), "no event may be published for a block without logs")
		require.Zero(t, receiptCalls.Load())
	})
}

func TestIsMethodNotFoundError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "code_32601", err: jsonrpc.Error{Code: -32601, Message: "anything"}, want: true},
		{name: "method_not_found_message", err: jsonrpc.Error{Code: -32000, Message: "Method not found"}, want: true},
		{name: "avalanche_public_rpc", err: jsonrpc.Error{Code: -32000, Message: "the method eth_getBlockReceipts does not exist/is not available"}, want: true},
		{name: "method_not_supported", err: jsonrpc.Error{Code: -32000, Message: "method eth_getBlockReceipts is not supported"}, want: true},
		{name: "wrapped", err: fmt.Errorf("batch: %w", jsonrpc.Error{Code: -32601, Message: "method not found"}), want: true},

		{name: "block_does_not_exist", err: jsonrpc.Error{Code: -32000, Message: "block 0xabc does not exist"}},
		{name: "header_not_found", err: jsonrpc.Error{Code: -32000, Message: "header not found"}},
		{name: "state_not_available", err: jsonrpc.Error{Code: -32000, Message: "historical state not available"}},
		{name: "unsupported_block_tag", err: jsonrpc.Error{Code: -32602, Message: "unsupported block number"}},
		{name: "timeout", err: jsonrpc.Error{Code: -32000, Message: "request timed out"}},
		{name: "http_method_not_found", err: superr.Wrap(ethrpc.ErrRequestFail, &jsonrpc.Error{Code: -32601, Message: "method not found"}), want: true},
		{name: "http_rate_limited", err: superr.Wrap(ethrpc.ErrRequestFail, &jsonrpc.Error{Code: -32005, Message: "rate limit exceeded"})},
		{name: "http_failure_without_body", err: superr.Wrap(ethrpc.ErrRequestFail, errors.New("non-200 response with status code: 503"))},
		{name: "plain_error", err: errors.New("method not found")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isMethodNotFoundError(tc.err))
		})
	}
}
