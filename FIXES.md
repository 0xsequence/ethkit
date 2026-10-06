# Receipt listener fixes

These changes correct receipt-listener findings and issues found during review. Receipt work remains owned by its exact filter registration, transaction, block hash and generation. Public filter values and custom matching remain intact.

- **Successful retry followed by cancellation.** A receipt fetch could succeed, lose its pending entry, then miss publication because the attempt expired while waiting for delivery. Pending removal now commits with publication under the delivery lock. A canceled attempt sends nothing and retains valid work for a prompt retry using the completed cache entry. RemoveFilter, ClearFilters and rollback remain terminal for the affected owner or candidate.
- **Partial callback completion.** A QueryOnChain callback may initially supply only TxHash. If its successful completion is retained after cancellation, the pending item now keeps the learned block/generation separately from its immutable queue identity. Rollback matches that completed candidate and releases its never-delivered LimitOne selection. Later retries validate the learned generation, so same-hash re-adoption cannot revive it. A fresh canonical transaction can be selected; an already-mined LimitOne transaction still keeps its selection through rollback and re-mining.
- **Stale RPC receipt after re-mining.** A provider can return the removed block A's receipt while the listener processes the same transaction in canonical block B. The invalid response is still rejected and never cached. Its rejection is retryable only when B's expected hash/generation remains current under the receipt lock, keeping B in the existing bounded pending queue. Obsolete generations and hash queries without a current expected block remain terminal. Mock-provider regressions cover initial processing and pending retries, recovery on an unrelated block, mined/final delivery, and obsolete work preserving a newer cache entry.
- **Same-hash re-adoption after retention eviction.** A delayed live Added event can outlast the monitor's retained chain. Receipt acceptance now uses positive evidence for that exact monitor incarnation, preserving valid adoption after deep eviction. Removed incarnations, cached registration snapshots and earlier RPC generations cannot reopen stale work. Missing hash lookup or height alone is insufficient; untracked/manual events require a positive retained canonical lookup.
- **Individual custom-value removal.** Comparable values and pointers retain Go equality. Otherwise unresolved non-comparable values of the same concrete type use a stable nonnil Exhausted channel as base identity. Shared-base values are aliases: removal chooses the first matching active registration, otherwise its first queued finality owner. Distinct bases or pointer identities provide independent removal, including through Filters() and Receipt.Filter copies. Nil/unstable signals cannot identify arbitrary callback value copies; register those as pointers for individual cancellation. Callback code pointers, unsafe identity and public filter IDs are not used as owner identity.

The monitor support belongs to [PR219](https://github.com/0xsequence/ethkit/pull/219); receipt changes remain in [PR220](https://github.com/0xsequence/ethkit/pull/220), based directly on parallel-monitor. The parent confirms cached parent mismatches even with local prefetch disabled and exposes Block.CanonicalState: acceptance is owned per incarnation, actual removal invalidates it, and retention eviction preserves it. This evidence covers known monitor removals; reorgs beyond retained known history remain the monitor's existing limit.

The parent's private Block state requires external positional literals to migrate to keyed literals, for example:

```go
ethmonitor.Block{Block: block, Event: ethmonitor.Added, Logs: logs, OK: true}
```

Keyed construction and the existing JSON schema remain supported; serialized blocks carry no runtime incarnation evidence. No parent implementation files are included in the receipt correction delta.

Final validation on actual synchronized sources:

- The two stale-RPC regression groups (four cases) passed five repetitions with the race detector: 1.656s. The same permanent tests fail against the pre-fix production source: current candidates are discarded and never retried, while obsolete-work controls pass.
- Full local receipt suite passed with the race detector: 254.839s, including all 41 permanent regression groups and existing integration tests. No overlay, chain reset or race warning.
- Parent monitor suite passed with the race detector: 73.007s. Only the external live-mainnet fee-history test was excluded; no external live RPC was used.
- go vet ./ethreceipts, go build ./..., Go formatting and diff checks passed for the stale-RPC follow-up. The unchanged monitor package retains its earlier passing vet and race checks.
