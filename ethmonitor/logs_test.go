package ethmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestMonitorEmptyLogResponses(t *testing.T) {
	cases := []struct {
		name         string
		payload      string
		nonzeroBloom bool
		filtered     bool
		status       int
		rpcError     bool
		emptyBody    bool
		ready        bool
	}{
		{name: "zero_bloom_array", payload: `[]`, ready: true},
		{name: "zero_bloom_null", payload: `null`, ready: true},
		{name: "zero_bloom_spaced_array", payload: `[ ]`, ready: true},
		{name: "nonzero_bloom_array", payload: `[]`, nonzeroBloom: true},
		{name: "nonzero_bloom_null", payload: `null`, nonzeroBloom: true},
		{name: "nonzero_bloom_spaced_array", payload: `[ ]`, nonzeroBloom: true},
		{name: "filtered_array", payload: `[]`, nonzeroBloom: true, filtered: true, ready: true},
		{name: "filtered_null", payload: `null`, nonzeroBloom: true, filtered: true, ready: true},
		{name: "filtered_spaced_array", payload: `[ ]`, nonzeroBloom: true, filtered: true, ready: true},
		{name: "invalid_object", payload: `{}`},
		{name: "invalid_log", payload: `[{}]`},
		{name: "http_error", payload: `null`, status: http.StatusServiceUnavailable},
		{name: "rpc_error", rpcError: true},
		{name: "empty_http_body", emptyBody: true},
		{name: "missing_rpc_result"},
	}
	for _, concurrency := range []int{0, 1} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("prefetch=%d/%s", concurrency, tc.name), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var request jsonrpc.Message
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					if request.Method != "eth_getLogs" {
						t.Errorf("unexpected RPC method %q", request.Method)
					}
					w.Header().Set("Content-Type", "application/json")
					status := tc.status
					if status == 0 {
						status = http.StatusOK
					}
					w.WriteHeader(status)
					if tc.emptyBody {
						return
					}
					response := jsonrpc.Message{Version: "2.0", ID: request.ID, Result: json.RawMessage(tc.payload)}
					if tc.rpcError {
						response.Error = &jsonrpc.Error{Code: -32000, Message: "provider not ready"}
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				provider, err := ethrpc.NewProvider(server.URL)
				require.NoError(t, err)
				opts := DefaultOptions
				opts.WithLogs = true
				opts.PrefetchConcurrency = concurrency
				if tc.filtered {
					opts.LogTopics = []common.Hash{common.HexToHash("0xabcd")}
				}
				monitor, err := NewMonitor(provider, opts)
				require.NoError(t, err)
				monitor.chainID = big.NewInt(1)
				hash := common.HexToHash("0x1000")
				var bloom types.Bloom
				if tc.nonzeroBloom {
					bloom = types.BytesToBloom([]byte{1}) // Logs exist, but need not match LogTopics.
				}
				for attempt := 0; attempt < 2; attempt++ {
					block := &Block{
						Block: types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1000), BlockHash: hash, Bloom: bloom}),
						Event: Added,
					}
					monitor.addLogs(context.Background(), Blocks{block})
					require.Equal(t, tc.ready, block.OK, "attempt %d", attempt+1)
					if tc.ready {
						require.NotNil(t, block.Logs, "accepted null must become an empty log slice")
						require.Empty(t, block.Logs)
					}
				}
				wantCalls := int32(2)
				if concurrency > 0 && tc.ready {
					wantCalls = 1 // Valid empty results are reused; failures remain retryable.
				} else if concurrency > 0 {
					wantCalls = 4 // Each cache error gets one direct origin retry.
				}
				require.Equal(t, wantCalls, calls.Load())
				if monitor.cache != nil {
					_, found, err := monitor.cache.Get(context.Background(), CacheKeyBlockLogs(monitor.chainID, hash, monitor.logTopics()))
					require.NoError(t, err)
					require.Equal(t, tc.ready, found, "only valid responses may remain cached")
				}
			})
		}
	}
}
