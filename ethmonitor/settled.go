package ethmonitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/goware/superr"
)

// hasSettledHeight reports whether a raw block payload carries a non-null
// top-level settledHeight field (ACP-194 / Avalanche Helicon).
func hasSettledHeight(payload []byte) bool {
	if !bytes.Contains(payload, []byte(`"settledHeight"`)) {
		return false
	}
	var header struct {
		SettledHeight json.RawMessage `json:"settledHeight"`
	}
	if err := json.Unmarshal(payload, &header); err != nil {
		return false
	}
	return len(header.SettledHeight) > 0 && !bytes.Equal(bytes.TrimSpace(header.SettledHeight), []byte("null"))
}

// receiptsBatchSize bounds each batch of per-transaction receipt calls, since
// RPC gateways and providers cap batch sizes (some reject batches over 40).
const receiptsBatchSize = 20

// errReceiptsFailed marks a filterLogs failure where the block's receipts
// were already fetched, and failed, in the same call.
var errReceiptsFailed = errors.New("ethmonitor: receipts failed")

// logsReceipt is the subset of a transaction receipt that receiptLogs needs.
// It is not decoded into types.Receipt, as chain-specific receipt fields may
// fail its strict decoding.
type logsReceipt struct {
	Type            hexutil.Uint64 `json:"type"`
	BlockHash       common.Hash    `json:"blockHash"`
	TransactionHash common.Hash    `json:"transactionHash"`
	Logs            []types.Log    `json:"logs"`
}

// receiptLogs returns the block's logs as recorded in its receipts, which,
// unlike eth_getLogs on settled-header chains, wait for execution. It is
// independent of the settled-header flag. Every failure returns an error.
func (m *Monitor) receiptLogs(ctx context.Context, block *types.Block) ([]types.Log, error) {
	txs := block.Transactions()
	blockHash := block.Hash()

	// ethrpc drops transaction types it can't decode, so an empty transaction
	// list only means an empty block when the header's root agrees.
	if len(txs) == 0 && block.TxHash() == types.EmptyTxsHash {
		return []types.Log{}, nil
	}

	if !m.blockReceiptsUnsupported.Load() {
		var receipts []*logsReceipt
		_, err := m.provider.Do(ctx, ethrpc.NewCallBuilder[[]*logsReceipt]("eth_getBlockReceipts", nil, blockHash).Into(&receipts))
		switch {
		case err == nil:
			if err := checkBlockReceipts(receipts, blockHash, len(txs)); err != nil {
				return nil, fmt.Errorf("ethmonitor: eth_getBlockReceipts for block %d %s: %w", block.NumberU64(), blockHash.Hex(), err)
			}
			return flattenReceiptLogs(receipts), nil
		case isMethodNotFoundError(err):
			// The node doesn't support the method, so stop asking. This lasts
			// for the monitor's lifetime: behind a load balancer with mixed
			// backends, one answer from a backend without the method moves
			// every later block to the per-tx fallback.
			if m.blockReceiptsUnsupported.CompareAndSwap(false, true) {
				m.log.Info(fmt.Sprintf("ethmonitor: eth_getBlockReceipts is not supported (%v), using eth_getTransactionReceipt from now on", err))
			}
		default:
			// Includes non-2xx responses such as rate limiting, which the
			// per-tx fallback would only add load to.
			return nil, fmt.Errorf("ethmonitor: eth_getBlockReceipts for block %d %s: %w", block.NumberU64(), blockHash.Hex(), err)
		}
	}

	// Fall back to one receipt per transaction, in batches of at most
	// receiptsBatchSize calls. The block's transaction count is fetched
	// alongside, as receipts can only be requested for transactions ethrpc
	// decoded.
	var txCount *hexutil.Uint
	receipts := make([]*logsReceipt, len(txs))
	calls := make([]ethrpc.Call, 0, len(txs)+1)
	calls = append(calls, ethrpc.NewCallBuilder[*hexutil.Uint]("eth_getBlockTransactionCountByHash", nil, blockHash).Into(&txCount))
	for i, tx := range txs {
		calls = append(calls, ethrpc.NewCallBuilder[*logsReceipt]("eth_getTransactionReceipt", nil, tx.Hash()).Into(&receipts[i]))
	}
	for start := 0; start < len(calls); start += receiptsBatchSize {
		end := min(start+receiptsBatchSize, len(calls))
		if _, err := m.provider.Do(ctx, calls[start:end]...); err != nil {
			return nil, fmt.Errorf("ethmonitor: eth_getTransactionReceipt for block %d %s: %w", block.NumberU64(), blockHash.Hex(), err)
		}
	}
	if txCount == nil {
		return nil, fmt.Errorf("ethmonitor: eth_getBlockTransactionCountByHash for block %d %s returned null", block.NumberU64(), blockHash.Hex())
	}
	if int(*txCount) != len(txs) {
		return nil, fmt.Errorf("ethmonitor: block %d %s has %d transactions but %d were decoded, per-transaction receipts would miss logs", block.NumberU64(), blockHash.Hex(), uint(*txCount), len(txs))
	}
	for i, receipt := range receipts {
		if receipt == nil {
			return nil, fmt.Errorf("ethmonitor: receipt for transaction %s in block %d %s is null (not executed yet)", txs[i].Hash().Hex(), block.NumberU64(), blockHash.Hex())
		}
		if receipt.BlockHash != blockHash {
			return nil, fmt.Errorf("ethmonitor: receipt for transaction %s has block hash %s, expected block %d %s", txs[i].Hash().Hex(), receipt.BlockHash.Hex(), block.NumberU64(), blockHash.Hex())
		}
	}
	return flattenReceiptLogs(receipts), nil
}

// checkBlockReceipts validates an eth_getBlockReceipts result against the
// block. Receipts beyond the decoded transactions are allowed only for
// transaction types ethrpc does not decode.
func checkBlockReceipts(receipts []*logsReceipt, blockHash common.Hash, numTxs int) error {
	// Only reached for blocks with transactions, so null (unknown or not yet
	// executed block) and [] are both errors.
	if len(receipts) == 0 {
		return fmt.Errorf("no receipts for a block with transactions")
	}
	decodable := 0
	for i, receipt := range receipts {
		if receipt == nil {
			return fmt.Errorf("receipt %d is null", i)
		}
		if receipt.BlockHash != blockHash {
			return fmt.Errorf("receipt %d has block hash %s", i, receipt.BlockHash.Hex())
		}
		if receipt.Type <= types.DynamicFeeTxType {
			decodable++
		}
	}
	if len(receipts) < numTxs || decodable > numTxs {
		return fmt.Errorf("%d receipts for %d transactions", len(receipts), numTxs)
	}
	return nil
}

// flattenReceiptLogs returns the receipts' logs in receipt order, which is
// logIndex order, as a non-nil slice.
func flattenReceiptLogs(receipts []*logsReceipt) []types.Log {
	logs := []types.Log{}
	for _, receipt := range receipts {
		logs = append(logs, receipt.Logs...)
	}
	return logs
}

// isMethodNotFoundError reports whether err is a JSON-RPC error saying the
// method isn't supported, in a response of any HTTP status: code -32601, or a
// message about the method itself, as some nodes answer with a generic code
// (e.g. "the method eth_getBlockReceipts does not exist/is not available").
// Any other JSON-RPC error may be transient (a timeout, rate limiting, a
// missing trie node, a lagging backend answering that a new block does not
// exist), so it is returned to the caller to be retried instead.
func isMethodNotFoundError(err error) bool {
	rpcErr, ok := jsonRPCError(err)
	if !ok {
		return false
	}
	if rpcErr.Code == -32601 {
		return true
	}
	msg := strings.ToLower(rpcErr.Message)
	if !strings.Contains(msg, "method") {
		return false
	}
	for _, s := range []string{"not found", "does not exist", "not supported", "unsupported", "not available"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// jsonRPCError returns the JSON-RPC error in err. ethrpc wraps the error body
// of a non-2xx response with superr, whose stack is not reachable through
// errors.As.
func jsonRPCError(err error) (jsonrpc.Error, bool) {
	var rpcErr jsonrpc.Error
	if errors.As(err, &rpcErr) {
		return rpcErr, true
	}
	for _, e := range superr.GetErrorStack(err) {
		switch e := e.(type) {
		case *jsonrpc.Error:
			return *e, true
		case jsonrpc.Error:
			return e, true
		}
	}
	return jsonrpc.Error{}, false
}
