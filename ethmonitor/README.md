# ethmonitor

Follows an EVM chain block by block and publishes each new block, and optionally its logs, to subscribers. Reorgs come through as explicit events.

ethmonitor keeps a short canonical chain in memory. When the node's view of the chain changes, it emits `Removed` events for the orphaned blocks and `Added` events for their replacements, in order, so a subscriber can keep its own state consistent with the chain.

## Usage

```go
provider, err := ethrpc.NewProvider("https://your-node-url")
if err != nil {
	return err
}

opts := ethmonitor.DefaultOptions
opts.WithLogs = true

monitor, err := ethmonitor.NewMonitor(provider, opts)
if err != nil {
	return err
}

go func() {
	if err := monitor.Run(ctx); err != nil {
		// Run returns when ctx is cancelled, Stop is called, or on a fatal error
	}
}()
defer monitor.Stop()

sub := monitor.Subscribe("my-service")
defer sub.Unsubscribe()

for {
	select {
	case blocks := <-sub.Blocks():
		for _, block := range blocks {
			switch block.Event {
			case ethmonitor.Added:
				// a new canonical block; block.Logs is set when WithLogs is on
			case ethmonitor.Removed:
				// a reorged block: undo whatever was done for its Added event
			}
		}
	case <-sub.Done():
		return sub.Err()
	}
}
```

To get new heads over a websocket instead of polling, pass `ethrpc.WithStreaming(wsURL)` to `ethrpc.NewProvider`. The monitor falls back to polling if streaming keeps failing (see [Options](#options)).

## How it works

```
 new head (poll or stream)
        │
        ▼
 fetch next block ──► canonical chain ──► fetch logs ──► publish queue ──► subscribers
                      (last N blocks,     (WithLogs)     (in order,
                       reorg detection)                   ready blocks only)
```

1. **Fetch.** The monitor fetches the next block by number.
2. **Chain.** If the block's parent is the current head, it's added to the in-memory chain. If not, the monitor walks back to the common ancestor, emitting `Removed` for each orphaned block, then adds the new branch. The chain keeps the last `BlockRetentionLimit` blocks.
3. **Logs.** With `WithLogs`, the block's logs are fetched with `eth_getLogs` by block hash and validated before the block counts as ready (`Block.OK`). See [Logs](#logs).
4. **Publish.** Events are queued and published **in order**, and publishing stops at the first block that isn't ready yet. With `TrailNumBlocksBehindHead`, events are held until they are that many blocks behind the head, which cuts down on `Added`/`Removed` churn on chains that reorg often.

### Events

Each `Blocks` batch a subscription receives holds one or more `*Block` events, oldest first:

- **`Added`:** the block joined the canonical chain.
- **`Removed`:** a block that was published as `Added` is no longer canonical. Match it by block hash.

`Blocks.Reorg()` reports whether a batch contains a removal. Consumers that store anything per block must handle `Removed`.

### Reading chain state

The monitor also answers questions about the chain it holds in memory:

- **Latest blocks:**
  - `LatestBlock()` returns the head of the in-memory chain.
  - `LatestReadyBlock()` returns the newest block whose data (including logs) is complete.
  - `LatestFinalBlock(n)` returns the block `n` blocks behind the head.
- **Lookups:** `GetBlock(hash)` and `GetTransaction(hash)` search the retained blocks.
- **Chain statistics:** `GetAverageBlockTime()`, and `FeeHistory(...)`, an approximation of `eth_feeHistory` computed from the retained blocks without fetching receipts. It uses gas limits and priority fees in place of gas used and effective priority fees.
- **The chain itself:** `Chain()` gives access to it directly.

## Logs

With `WithLogs`, every published `Added` block carries its logs in `Block.Logs`. `LogTopics` limits them to logs whose first topic is in the list.

### How logs are validated

A node can answer `eth_getLogs` with `[]` before it has indexed a block, so an empty result isn't always trusted:

- **Most chains:** if `eth_getLogs` returns `[]` but the header's `logsBloom` is non-zero, the result is rejected and retried, since the bloom says the block has logs.
- **Chains with settled headers:** on Avalanche C-Chain after ACP-194 (Helicon), the `logsBloom` describes the *settled* blocks rather than the block itself, so it can't be used. The monitor detects these chains from the headers' `settledHeight` field, with no configuration needed. It confirms an empty result against the block's receipts, which a node only serves once the block has executed. If the receipts have logs, those are used.
- **Receipts methods:** receipts are fetched with `eth_getBlockReceipts`. If the node says the method doesn't exist, the monitor switches to `eth_getTransactionReceipt` per transaction, in batches of 20, plus `eth_getBlockTransactionCountByHash`. It uses that path for the rest of its lifetime.

### When logs keep failing

A block whose logs can't be fetched blocks everything after it, because publishing is in order. To keep the monitor moving:

1. **Receipts fallback.** After `MaxLogsAttempts` failed attempts (default 10), the block's logs are taken from its receipts. On chains without settled headers, empty receipts for a block with a non-zero bloom are still rejected.
2. **Leaving the chain cache.** If the block is still unresolved when it leaves the chain cache (`BlockRetentionLimit` blocks later), one more receipts attempt is made. If that fails too:
   - **Default:** the block is published with empty `Logs` and **`Block.LogsUnconfirmed = true`**. An Error is logged and the `Alerter` is called.
   - **`StrictLogs: true`:** the block is never published without confirmed logs. The monitor keeps retrying, and when the publish queue (2 × `BlockRetentionLimit` events) overflows, `Run` returns `ErrFatal` wrapping `ErrQueueFull`.

**Which mode to use:** keep the default when a short gap is better than downtime, for example live notifications. Use `StrictLogs` when a missing log is worse than stopping. For example, an indexer that persists what it receives would rather restart from its last checkpoint than record a block as having no events.

## Options

Start from `ethmonitor.DefaultOptions` and override what you need.

| Option | Default | What it does |
|---|---|---|
| `PollingInterval` | 1.5s | How often to poll for new blocks. |
| `StreamingDisabled` | false | Poll even if the provider has a websocket URL. |
| `StreamingErrNumToSwitchToPolling` | 3 | Streaming errors before switching to polling. |
| `StreamingErrorResetInterval` | 2m | How often the streaming error count resets. |
| `StreamingRetryAfter` | 5m | How long to poll before retrying streaming. |
| `Timeout` | 20s | Timeout for node requests. |
| `StartBlockNumber` | nil (latest) | Block to start from. A negative value starts that many blocks behind the latest. |
| `Bootstrap` | false | Start from a chain restored with `Chain().BootstrapFromBlocks` / `BootstrapFromBlocksJSON`. Takes precedence over `StartBlockNumber`. |
| `TrailNumBlocksBehindHead` | 0 | Publish only once a block is this many blocks behind the head. |
| `BlockRetentionLimit` | 200 | Blocks kept in the in-memory chain. This is also the deepest reorg the monitor can follow. |
| `WithLogs` | false | Fetch and publish each block's logs. |
| `LogTopics` | all | Only include logs whose first topic is in this list. |
| `MaxLogsAttempts` | 10 | Failed `eth_getLogs` attempts before falling back to receipts. |
| `StrictLogs` | false | Never publish a block without confirmed logs (see [Logs](#logs)). |
| `PrefetchConcurrency` | 0 (off) | Workers fetching blocks and logs ahead of the monitor while it trails the head. Useful on fast chains. |
| `PrefetchWindow` | 4 × concurrency | How far ahead prefetching may go. |
| `CacheBackend` | nil | Shared cache for block and log payloads. In most cases, leave it nil. |
| `CacheExpiry` | 10m | Lifetime of cache entries. |
| `UnsubscribeOnStop` | false | Close all subscriptions when `Run` returns. |
| `ChainID` | from node | Set it to skip asking the node. |
| `Alerter` | no-op | Called on conditions that need attention, such as a block published without confirmed logs. |
| `Logger` | discard | `*slog.Logger` for warnings and debug output. |
| `DebugLogging` | false | Verbose logging. |

## Restarts and snapshots

`Chain().Snapshot()` serializes the retained chain to JSON. A new monitor created with `Bootstrap: true` can then be restored with `Chain().BootstrapFromBlocksJSON(data)` (or `BootstrapFromBlocks`) before `Run`. It continues from the snapshot's head and can still follow reorgs into the restored blocks.

## Errors

`Run` returns when its context is cancelled, when `Stop` is called, or on an error the monitor can't recover from. Fatal errors wrap `ErrFatal`. `ErrQueueFull` is the main one: the publish queue overflowed. In default mode that only happens for reorgs deeper than the chain cache, or with very small retention.

A subscription's `Done()` channel closes when it ends, and `Err()` returns the reason.

## Known limitations

- **Topic filters on settled-header chains.** With `LogTopics` set, an empty result isn't confirmed against receipts, so a premature empty result on these chains is accepted.
- **Undecoded transaction types.** ethrpc skips transaction types it can't decode. With `eth_getBlockReceipts`, a partial response missing the receipt of such a transaction isn't detected.
- **Very small retention.** With `BlockRetentionLimit + TrailNumBlocksBehindHead <= 5`, the publish queue can overflow even in default mode.
