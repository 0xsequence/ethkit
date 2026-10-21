package ethmonitor

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// resolveFromReceipts sets the block's logs from its receipts, filtered by the
// monitor's log topics, and marks it ready. It doesn't write to the cache. On
// failure the block is left unchanged.
func (m *Monitor) resolveFromReceipts(ctx context.Context, block *Block) error {
	tctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	logs, err := m.receiptLogs(tctx, block.Block)
	if err != nil {
		return err
	}

	// Outside settled-header chains the header bloom covers the block's own
	// logs, so receipts without any logs contradict a non-zero bloom, the same
	// way an empty eth_getLogs result does.
	if len(logs) == 0 && !m.usesSettledHeaders() && block.Bloom() != (types.Bloom{}) {
		return fmt.Errorf("ethmonitor: block %d %s: receipts have no logs but block bloom is set", block.NumberU64(), block.Hash().Hex())
	}

	// apply the same topic filter as the eth_getLogs query
	topics := m.logTopics()
	if len(topics) > 0 {
		filtered := make([]types.Log, 0, len(logs))
		for _, log := range logs {
			if matchTopics(log.Topics, topics) {
				filtered = append(filtered, log)
			}
		}
		logs = filtered
	}

	block.Logs = logs
	block.OK = true

	m.log.Warn(fmt.Sprintf("ethmonitor: block %d %s: eth_getLogs failed %d times, using logs from receipts", block.NumberU64(), block.Hash().Hex(), block.logsAttempts))
	return nil
}

// matchTopics reports whether log topics match an eth_getLogs topic filter,
// as geth's filter does: each position matches any of its values, an empty
// position matches anything, and the log needs a topic for every position.
func matchTopics(logTopics []common.Hash, topics [][]common.Hash) bool {
	if len(topics) > len(logTopics) {
		return false
	}
	for i, want := range topics {
		if len(want) > 0 && !slices.Contains(want, logTopics[i]) {
			return false
		}
	}
	return true
}

// resolveEvictedBlocks resolves queued blocks whose logs are still missing and
// which have left the chain cache, where backfillChainLogs no longer retries
// them. They are taken from receipts, or, unless StrictLogs is set, published
// with empty logs, so they can't hold up the publish queue.
//
// NOTE: called with m.chain.mu held, so it reads m.chain.blocks directly and
// must not call Chain methods which take the lock.
func (m *Monitor) resolveEvictedBlocks(ctx context.Context) {
	if len(m.chain.blocks) == 0 {
		return
	}
	oldest := m.chain.blocks[0].NumberU64()

	for _, block := range m.publishQueue.unresolved(oldest) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// A reorg deeper than the chain cache can pop a queued block below the
		// oldest retained one. Its Removed event, published in this same loop
		// iteration, drops it from the queue, so leave it alone.
		if _, canonical := block.CanonicalState(); !canonical {
			continue
		}

		err := m.resolveFromReceipts(ctx, block)
		if err == nil {
			continue
		}

		if m.options.StrictLogs {
			// Blocks are published in order, so later ones can't be published
			// before this one is resolved. Retry on the next iteration.
			return
		}

		block.Logs = []types.Log{}
		block.OK = true

		m.log.Error(fmt.Sprintf("ethmonitor: block %d %s: publishing without logs, eth_getLogs and receipts both failed after %d attempts: %v", block.NumberU64(), block.Hash().Hex(), block.logsAttempts, err))
		if m.alert != nil {
			m.alert.Alert(ctx, "ethmonitor: block %d %s: publishing without logs, eth_getLogs and receipts both failed after %d attempts: %v", block.NumberU64(), block.Hash().Hex(), block.logsAttempts, err)
		}
	}
}
