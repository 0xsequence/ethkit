package ethmonitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
	memcache "github.com/goware/cachestore-mem"
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

// testNode is an httptest JSON-RPC node which counts the calls it serves per
// method. Batch requests are served call by call and counted the same way.
type testNode struct {
	*httptest.Server
	mu     sync.Mutex
	counts map[string]int

	maxBatch     int            // when set, larger batches are rejected with HTTP 400, as some RPC gateways do
	status       map[string]int // HTTP status for single requests to a method, 200 by default
	largestBatch int
}

func newTestNode(t *testing.T, handle func(method string, params []any) (json.RawMessage, *jsonrpc.Error)) *testNode {
	t.Helper()
	node := &testNode{counts: map[string]int{}}
	node.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		if _, err := body.ReadFrom(r.Body); err != nil {
			t.Error(err)
			return
		}
		var requests []jsonrpc.Message
		batch := bytes.HasPrefix(bytes.TrimSpace(body.Bytes()), []byte("["))
		if batch {
			if err := json.Unmarshal(body.Bytes(), &requests); err != nil {
				t.Error(err)
				return
			}
		} else {
			var request jsonrpc.Message
			if err := json.Unmarshal(body.Bytes(), &request); err != nil {
				t.Error(err)
				return
			}
			requests = append(requests, request)
		}

		node.mu.Lock()
		node.largestBatch = max(node.largestBatch, len(requests))
		maxBatch := node.maxBatch
		node.mu.Unlock()
		if maxBatch > 0 && len(requests) > maxBatch {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(jsonrpc.Message{Version: "2.0", Error: &jsonrpc.Error{Code: -32600, Message: fmt.Sprintf("too many batch requests, max is %d", maxBatch)}}); err != nil {
				t.Error(err)
			}
			return
		}

		responses := make([]jsonrpc.Message, 0, len(requests))
		for _, request := range requests {
			node.mu.Lock()
			node.counts[request.Method]++
			node.mu.Unlock()
			result, rpcErr := handle(request.Method, request.Params)
			responses = append(responses, jsonrpc.Message{Version: "2.0", ID: request.ID, Result: result, Error: rpcErr})
		}

		w.Header().Set("Content-Type", "application/json")
		node.mu.Lock()
		status := node.status[requests[0].Method]
		node.mu.Unlock()
		if !batch && status != 0 {
			w.WriteHeader(status)
		}
		var err error
		if batch {
			err = json.NewEncoder(w).Encode(responses)
		} else {
			err = json.NewEncoder(w).Encode(responses[0])
		}
		if err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(node.Close)
	return node
}

func (n *testNode) calls(method string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.counts[method]
}

// receiptsBehavior configures what a receiptsNode answers.
type receiptsBehavior struct {
	logs        json.RawMessage // eth_getLogs result, [] by default
	logsErr     *jsonrpc.Error  // eth_getLogs JSON-RPC error
	receiptsErr *jsonrpc.Error  // JSON-RPC error from every receipts method
	blockRcpErr *jsonrpc.Error  // JSON-RPC error from eth_getBlockReceipts only

	receiptLogs      map[common.Hash][]types.Log // receipt logs by block hash, held by the first tx's receipt
	wrongBlockHash   bool                        // receipts carry another block's hash
	nullTxReceipt    bool                        // eth_getTransactionReceipt returns null
	dropReceipts     int                         // eth_getBlockReceipts omits this many receipts
	swapLastForTyped bool                        // eth_getBlockReceipts replaces the last tx's receipt with one for an undecoded tx type
	txCountDelta     int                         // eth_getBlockTransactionCountByHash is off by this much
}

var errReceiptsDown = &jsonrpc.Error{Code: -32000, Message: "receipts down"}

// newReceiptsNode serves eth_getLogs and the receipts methods for blocks.
func newReceiptsNode(t *testing.T, blocks []*types.Block, b *receiptsBehavior) *testNode {
	t.Helper()
	byHash := map[common.Hash]*types.Block{}
	byTx := map[common.Hash]struct {
		block *types.Block
		index int
	}{}
	for _, block := range blocks {
		byHash[block.Hash()] = block
		for i, tx := range block.Transactions() {
			byTx[tx.Hash()] = struct {
				block *types.Block
				index int
			}{block, i}
		}
	}

	receipt := func(block *types.Block, i int) map[string]any {
		blockHash := block.Hash()
		if b.wrongBlockHash {
			blockHash = common.HexToHash("0xdead")
		}
		logs := []types.Log{}
		if i == 0 {
			logs = b.receiptLogs[block.Hash()]
		}
		return map[string]any{
			"type":            "0x0",
			"blockHash":       blockHash,
			"transactionHash": block.Transactions()[i].Hash(),
			"logs":            logs,
		}
	}
	hashParam := func(params []any) common.Hash {
		s, _ := params[0].(string)
		return common.HexToHash(s)
	}

	return newTestNode(t, func(method string, params []any) (json.RawMessage, *jsonrpc.Error) {
		switch method {
		case "eth_getLogs":
			if b.logsErr != nil {
				return nil, b.logsErr
			}
			if b.logs != nil {
				return b.logs, nil
			}
			return json.RawMessage(`[]`), nil
		case "eth_getBlockReceipts", "eth_getTransactionReceipt", "eth_getBlockTransactionCountByHash":
			if b.receiptsErr != nil {
				return nil, b.receiptsErr
			}
		}

		var result any
		switch method {
		case "eth_getBlockReceipts":
			if b.blockRcpErr != nil {
				return nil, b.blockRcpErr
			}
			block, ok := byHash[hashParam(params)]
			if !ok {
				return json.RawMessage(`null`), nil
			}
			receipts := []map[string]any{}
			for i := 0; i < len(block.Transactions())-b.dropReceipts; i++ {
				receipts = append(receipts, receipt(block, i))
			}
			if b.swapLastForTyped && len(receipts) > 0 {
				receipts[len(receipts)-1] = map[string]any{
					"type":            "0x3",
					"blockHash":       block.Hash(),
					"transactionHash": common.HexToHash("0xb10b"),
					"logs":            []types.Log{},
				}
			}
			result = receipts
		case "eth_getTransactionReceipt":
			found, ok := byTx[hashParam(params)]
			if !ok || b.nullTxReceipt {
				return json.RawMessage(`null`), nil
			}
			result = receipt(found.block, found.index)
		case "eth_getBlockTransactionCountByHash":
			block, ok := byHash[hashParam(params)]
			if !ok {
				return json.RawMessage(`null`), nil
			}
			result = hexutil.Uint(len(block.Transactions()) + b.txCountDelta)
		default:
			t.Errorf("unexpected RPC method %q", method)
			return nil, &jsonrpc.Error{Code: -32601, Message: "method not found"}
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Error(err)
			return nil, &jsonrpc.Error{Code: -32603, Message: err.Error()}
		}
		return encoded, nil
	})
}

var testSigningKey, _ = crypto.HexToECDSA("4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318")

// newTxBlock builds a block with txCount signed transactions. Its header
// TxHash is the empty root without transactions and non-zero with them, as a
// real one would be.
func newTxBlock(t *testing.T, number uint64, txCount int, bloom types.Bloom) *types.Block {
	t.Helper()
	signer := types.LatestSignerForChainID(big.NewInt(1))
	txs := make([]*types.Transaction, 0, txCount)
	for i := 0; i < txCount; i++ {
		tx, err := types.SignTx(types.NewTx(&types.LegacyTx{
			Nonce:    number*100 + uint64(i),
			GasPrice: big.NewInt(1),
			Gas:      21000,
			To:       &common.Address{1},
			Value:    big.NewInt(1),
		}), signer, testSigningKey)
		require.NoError(t, err)
		txs = append(txs, tx)
	}
	header := &types.Header{
		Number:     new(big.Int).SetUint64(number),
		ParentHash: crypto.Keccak256Hash(new(big.Int).SetUint64(number - 1).Bytes()),
		Difficulty: big.NewInt(0),
		Bloom:      bloom,
	}
	header.SetHash(crypto.Keccak256Hash(new(big.Int).SetUint64(number).Bytes()))
	header.TxHash = types.EmptyTxsHash
	if txCount > 0 {
		header.TxHash = common.HexToHash("0xbeef")
	}
	return types.NewBlockWithHeader(header).WithBody(types.Body{Transactions: txs})
}

// newTestLog builds a log of block's first transaction, in the shape a node returns.
func newTestLog(block *types.Block, index uint, topic common.Hash) types.Log {
	return types.Log{
		Address:     common.HexToAddress("0x1234"),
		Topics:      []common.Hash{topic},
		Data:        []byte{1, 2, 3},
		BlockNumber: block.NumberU64(),
		TxHash:      block.Transactions()[0].Hash(),
		BlockHash:   block.Hash(),
		Index:       index,
	}
}

var nonzeroBloom = types.BytesToBloom([]byte{1})

func TestSettledEmptyLogResponses(t *testing.T) {
	topicA := common.HexToHash("0xaa")

	type counts struct{ getLogs, blockReceipts, txCount, txReceipt int }
	cases := []struct {
		name     string
		settled  bool // when false, the monitor is not a settled-header one
		bloom    types.Bloom
		txs      int
		blocks   int // blocks to run addLogs for, 1 by default
		cache    bool
		numLogs  int // receipt logs per block
		logs     string
		behavior receiptsBehavior
		maxBatch int            // node batch limit, such as an RPC gateway's 40
		status   map[string]int // HTTP status by method, for single requests
		preCache string         // logs cache entry written beforehand, e.g. by an older monitor
		ready    bool
		want     counts // totals across all blocks
	}{
		{name: "a_nonzero_bloom_empty_no_txs", settled: true, bloom: nonzeroBloom, ready: true,
			want: counts{getLogs: 1}},
		{name: "b_nonzero_bloom_empty_two_txs_no_receipt_logs", settled: true, bloom: nonzeroBloom, txs: 2, ready: true,
			want: counts{getLogs: 1, blockReceipts: 1}},
		{name: "c_zero_bloom_receipts_have_logs", settled: true, txs: 1, numLogs: 2, ready: true,
			want: counts{getLogs: 1, blockReceipts: 1}},
		{name: "c_zero_bloom_receipts_have_logs_cached", settled: true, txs: 1, numLogs: 2, cache: true, ready: true,
			want: counts{getLogs: 1, blockReceipts: 1}},
		{name: "d_block_receipts_unsupported_falls_back_and_is_memoised", settled: true, txs: 2, blocks: 2, ready: true,
			behavior: receiptsBehavior{blockRcpErr: &jsonrpc.Error{Code: -32601, Message: "method not found"}},
			want:     counts{getLogs: 2, blockReceipts: 1, txCount: 2, txReceipt: 4}},
		{name: "e_receipt_block_hash_mismatch", settled: true, txs: 1, numLogs: 1,
			behavior: receiptsBehavior{wrongBlockHash: true},
			want:     counts{getLogs: 1, blockReceipts: 1}},
		{name: "e_tx_receipt_block_hash_mismatch", settled: true, txs: 1, numLogs: 1,
			behavior: receiptsBehavior{wrongBlockHash: true, blockRcpErr: &jsonrpc.Error{Code: -32601, Message: "method not found"}},
			want:     counts{getLogs: 1, blockReceipts: 1, txCount: 1, txReceipt: 1}},
		{name: "f_null_tx_receipt", settled: true, txs: 2,
			behavior: receiptsBehavior{nullTxReceipt: true, blockRcpErr: &jsonrpc.Error{Code: -32601, Message: "method not found"}},
			want:     counts{getLogs: 1, blockReceipts: 1, txCount: 1, txReceipt: 2}},
		{name: "g_logs_returned_no_receipt_calls", settled: true, bloom: nonzeroBloom, txs: 1, numLogs: 1, ready: true,
			logs: "log", want: counts{getLogs: 1}},
		{name: "h_not_settled_nonzero_bloom_empty", bloom: nonzeroBloom, txs: 1, numLogs: 1,
			want: counts{getLogs: 1}},
		{name: "i_block_receipts_count_below_tx_count", settled: true, txs: 2,
			behavior: receiptsBehavior{dropReceipts: 1},
			want:     counts{getLogs: 1, blockReceipts: 1}},
		{name: "i_block_receipts_missing_decoded_tx_with_typed_receipt", settled: true, txs: 2, numLogs: 1,
			behavior: receiptsBehavior{swapLastForTyped: true},
			want:     counts{getLogs: 1, blockReceipts: 1}},
		{name: "i_tx_count_differs_from_decoded_txs", settled: true, txs: 1,
			behavior: receiptsBehavior{txCountDelta: 1, blockRcpErr: &jsonrpc.Error{Code: -32601, Message: "method not found"}},
			want:     counts{getLogs: 1, blockReceipts: 1, txCount: 1, txReceipt: 1}},
		{name: "receipts_unavailable", settled: true, txs: 1,
			behavior: receiptsBehavior{receiptsErr: errReceiptsDown},
			want:     counts{getLogs: 1, blockReceipts: 1}},
		{name: "d_transient_block_receipts_error_is_not_memoised", settled: true, txs: 2, blocks: 2,
			behavior: receiptsBehavior{blockRcpErr: &jsonrpc.Error{Code: -32000, Message: "request timed out"}},
			want:     counts{getLogs: 2, blockReceipts: 2}},
		{name: "d_fallback_batches_stay_within_gateway_limit", settled: true, txs: 45, numLogs: 2, maxBatch: 40, ready: true,
			behavior: receiptsBehavior{blockRcpErr: &jsonrpc.Error{Code: -32601, Message: "method not found"}},
			want:     counts{getLogs: 1, blockReceipts: 1, txCount: 1, txReceipt: 45}},
		{name: "d_block_not_found_message_is_not_memoised", settled: true, txs: 2, blocks: 2,
			behavior: receiptsBehavior{blockRcpErr: &jsonrpc.Error{Code: -32000, Message: "block does not exist"}},
			want:     counts{getLogs: 2, blockReceipts: 2}},
		{name: "d_rate_limited_block_receipts_is_retried_without_fallback", settled: true, txs: 2, blocks: 2,
			status:   map[string]int{"eth_getBlockReceipts": http.StatusTooManyRequests},
			behavior: receiptsBehavior{blockRcpErr: &jsonrpc.Error{Code: -32005, Message: "rate limit exceeded"}},
			want:     counts{getLogs: 2, blockReceipts: 2}},
		{name: "d_http_method_not_found_is_memoised", settled: true, txs: 2, blocks: 2, ready: true,
			status:   map[string]int{"eth_getBlockReceipts": http.StatusBadRequest},
			behavior: receiptsBehavior{blockRcpErr: &jsonrpc.Error{Code: -32601, Message: "method not found"}},
			want:     counts{getLogs: 2, blockReceipts: 1, txCount: 2, txReceipt: 4}},
		{name: "j_premature_empty_cached_by_peer_is_replaced", settled: true, txs: 1, numLogs: 2, cache: true, preCache: `[]`, ready: true,
			want: counts{blockReceipts: 1}},
		{name: "j_premature_empty_cached_by_peer_receipts_unavailable", settled: true, txs: 1, numLogs: 2, preCache: `[]`,
			behavior: receiptsBehavior{receiptsErr: errReceiptsDown},
			want:     counts{blockReceipts: 1}},
		{name: "d_method_unavailable_message_is_memoised", settled: true, txs: 2, blocks: 2, ready: true,
			behavior: receiptsBehavior{blockRcpErr: &jsonrpc.Error{Code: -32000, Message: "the method eth_getBlockReceipts does not exist/is not available"}},
			want:     counts{getLogs: 2, blockReceipts: 1, txCount: 2, txReceipt: 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.blocks == 0 {
				tc.blocks = 1
			}
			blocks := make([]*types.Block, 0, tc.blocks)
			behavior := tc.behavior
			behavior.receiptLogs = map[common.Hash][]types.Log{}
			wantLogs := map[common.Hash][]types.Log{}
			for i := 0; i < tc.blocks; i++ {
				block := newTxBlock(t, 1000+uint64(i), tc.txs, tc.bloom)
				blocks = append(blocks, block)
				var logs []types.Log
				for j := 0; j < tc.numLogs; j++ {
					logs = append(logs, newTestLog(block, uint(j), topicA))
				}
				behavior.receiptLogs[block.Hash()] = logs
				wantLogs[block.Hash()] = logs
			}
			if tc.logs == "log" {
				payload, err := json.Marshal(wantLogs[blocks[0].Hash()])
				require.NoError(t, err)
				behavior.logs = payload
			}

			node := newReceiptsNode(t, blocks, &behavior)
			node.maxBatch = tc.maxBatch
			node.status = tc.status
			provider, err := ethrpc.NewProvider(node.URL)
			require.NoError(t, err)
			opts := DefaultOptions
			opts.WithLogs = true
			if tc.cache || tc.preCache != "" {
				backend, err := memcache.NewBackend(64)
				require.NoError(t, err)
				opts.CacheBackend = backend
			}
			monitor, err := NewMonitor(provider, opts)
			require.NoError(t, err)
			monitor.chainID = big.NewInt(1)
			monitor.settledHeaders.Store(tc.settled)
			if tc.preCache != "" {
				for _, block := range blocks {
					key := CacheKeyBlockLogs(monitor.chainID, block.Hash(), monitor.logTopics())
					require.NoError(t, monitor.cache.Set(context.Background(), key, []byte(tc.preCache)))
				}
			}

			for _, block := range blocks {
				b := &Block{Block: block, Event: Added}
				monitor.addLogs(context.Background(), Blocks{b})
				require.Equal(t, tc.ready, b.OK)
				if !tc.ready {
					require.Nil(t, b.Logs)
					continue
				}
				require.NotNil(t, b.Logs)
				if want := wantLogs[block.Hash()]; len(want) == 0 {
					require.Empty(t, b.Logs)
				} else {
					require.Equal(t, want, b.Logs)
				}
			}

			got := counts{
				getLogs:       node.calls("eth_getLogs"),
				blockReceipts: node.calls("eth_getBlockReceipts"),
				txCount:       node.calls("eth_getBlockTransactionCountByHash"),
				txReceipt:     node.calls("eth_getTransactionReceipt"),
			}
			require.Equal(t, tc.want, got)
			if tc.maxBatch > 0 {
				node.mu.Lock()
				require.LessOrEqual(t, node.largestBatch, receiptsBatchSize)
				node.mu.Unlock()
			}

			if tc.cache {
				// the receipt logs were cached, so a second read replays them
				// without asking the node
				logs, _, err := monitor.filterLogs(context.Background(), blocks[0], monitor.logTopics())
				require.NoError(t, err)
				require.Equal(t, wantLogs[blocks[0].Hash()], logs)
				require.Equal(t, tc.want.getLogs, node.calls("eth_getLogs"))
				require.Equal(t, tc.want.blockReceipts, node.calls("eth_getBlockReceipts"))
			}
		})
	}
}
