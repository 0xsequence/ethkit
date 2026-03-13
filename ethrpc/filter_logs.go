package ethrpc

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// maxFilterLogsBlockRange is the hard upper limit on the total block range that
// filterLogsAutoSplit will process. This prevents callers from accidentally
// issuing hundreds of sub-queries for unreasonably large ranges (e.g. 0 to 1M).
// Callers needing larger ranges should paginate at the application level.
const maxFilterLogsBlockRange = uint64(10_000)

// filterLogs executes a standard eth_getLogs JSON-RPC call for the given query.
func (p *Provider) filterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	var logs []types.Log
	_, err := p.Do(ctx, FilterLogs(q).Strict(p.strictness).Into(&logs))
	return logs, err
}

// filterLogsAutoSplit executes eth_getLogs with automatic range splitting when the node
// returns a "too much data" style error. It uses an AIMD (Additive Increase,
// Multiplicative Decrease) strategy to adapt the batch range over time.
//
// Calibration: on the first call the provider tries the full range (or the explicit max).
// If the node rejects it, we shrink (multiplicative decrease /1.5) and retry. Within a
// single call, after each successful sub-query we attempt additive increase (+10%). If
// the increase triggers another error, we shrink back and mark the range as "calibrated"
// — meaning the node's true limit has been found. Once calibrated, subsequent calls skip
// the probing entirely and use the known-good range directly.
func (p *Provider) filterLogsAutoSplit(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	from, to, err := p.resolveFilterBlockRange(ctx, q)
	if err != nil {
		return nil, err
	}

	// Single block: just do a direct call. Empty/invalid range: return early.
	if from == to {
		return p.filterLogs(ctx, q)
	}
	if from > to {
		return nil, nil
	}

	totalRange := to - from

	// Safety limit: reject unreasonably large ranges to prevent hammering the node
	// with hundreds of sub-queries. Callers should paginate at the application level.
	if totalRange > maxFilterLogsBlockRange {
		return nil, fmt.Errorf("ethrpc: FilterLogs block range of %d exceeds maximum of %d", totalRange, maxFilterLogsBlockRange)
	}
	batchRange := p.effectiveFilterLogsBatchRange(totalRange)

	// Additive factor: 10% of the starting batch range, minimum 1
	additiveFactor := uint64(math.Ceil(float64(batchRange) * 0.10))
	if additiveFactor < 1 {
		additiveFactor = 1
	}

	// Track whether we've succeeded after a shrink. We only mark calibrated
	// on a shrink→success→grow→fail cycle, which means we've bracketed the
	// true limit. A shrink→shrink sequence (consecutive failures) does not
	// count as calibration.
	succeededSinceShrink := false

	var allLogs []types.Log

	for cursor := from; cursor <= to; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		end := cursor + batchRange
		if end > to {
			end = to
		}

		subQ := q // copy value (FilterQuery is a struct)
		subQ.FromBlock = new(big.Int).SetUint64(cursor)
		subQ.ToBlock = new(big.Int).SetUint64(end)

		logs, err := p.filterLogs(ctx, subQ)
		if err != nil {
			if !isFilterLogsTooMuchDataError(err) {
				return nil, err
			}

			// If we previously grew after a successful shrink and now failed,
			// we've bracketed the node's true limit. Mark calibrated.
			if succeededSinceShrink {
				p.filterLogsRangeCalibrated.Store(true)
			}
			succeededSinceShrink = false

			// Multiplicative decrease: shrink by /1.5
			batchRange = uint64(float64(batchRange) / 1.5)
			if batchRange < 1 {
				batchRange = 1
			}

			continue // retry the same cursor with a smaller range
		}

		allLogs = append(allLogs, logs...)
		succeededSinceShrink = true

		// Store what worked
		p.filterLogsLastRange.Store(int64(batchRange))

		// Advance past the successfully fetched range
		cursor = end + 1

		// Check context before continuing to next batch
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Additive increase: try to grow the batch range if not yet calibrated
		if cursor <= to && !p.filterLogsRangeCalibrated.Load() {
			ceiling := p.filterLogsCeiling()
			grown := batchRange + additiveFactor
			if grown > ceiling {
				grown = ceiling
			}
			batchRange = grown
		}
	}

	return allLogs, nil
}

// resolveFilterBlockRange resolves the actual uint64 from/to block numbers from a
// FilterQuery, fetching the latest block number if ToBlock is nil.
func (p *Provider) resolveFilterBlockRange(ctx context.Context, q ethereum.FilterQuery) (uint64, uint64, error) {
	var from uint64
	if q.FromBlock != nil {
		from = q.FromBlock.Uint64()
	}

	if q.ToBlock != nil {
		return from, q.ToBlock.Uint64(), nil
	}

	// ToBlock is nil → fetch latest block number
	latest, err := p.BlockNumber(ctx)
	if err != nil {
		return 0, 0, err
	}
	return from, latest, nil
}

// effectiveFilterLogsBatchRange picks the starting batch range for a filterLogsAutoSplit call.
func (p *Provider) effectiveFilterLogsBatchRange(totalRange uint64) uint64 {
	// If we have a previously discovered value, use it directly
	if last := p.filterLogsLastRange.Load(); last > 0 {
		v := uint64(last)
		// Respect the explicit ceiling if set
		if p.filterLogsMaxRange > 0 && v > uint64(p.filterLogsMaxRange) {
			v = uint64(p.filterLogsMaxRange)
		}
		if v > totalRange {
			return totalRange
		}
		return v
	}

	// Not yet calibrated: use explicit max if set
	if p.filterLogsMaxRange > 0 {
		if uint64(p.filterLogsMaxRange) < totalRange {
			return uint64(p.filterLogsMaxRange)
		}
		return totalRange
	}

	// Auto mode (filterLogsMaxRange == 0), not yet calibrated: try the full range
	return totalRange
}

// filterLogsCeiling returns the upper bound for additive increase.
func (p *Provider) filterLogsCeiling() uint64 {
	if p.filterLogsMaxRange > 0 {
		return uint64(p.filterLogsMaxRange)
	}
	// Auto mode: no artificial ceiling, let AIMD discover it
	return math.MaxUint64
}

// isFilterLogsTooMuchDataError checks whether an error from the node indicates
// the requested range or result set was too large.
func isFilterLogsTooMuchDataError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, pattern := range filterLogsTooMuchDataPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// filterLogsTooMuchDataPatterns are lowercase substrings matched against error messages
// from various RPC node providers to detect "too much data" / "block range too large" errors.
var filterLogsTooMuchDataPatterns = []string{
	"query returned more than",      // Infura, Alchemy, generic (e.g. 10000 results)
	"query exceeds max results",     // Telos
	"response is too big",           // Soneium
	"response exceed size limit",    // Various
	"log response size exceeded",    // Various
	"block range",                   // Catches "block range too large", "block range exceeded", etc.
	"too many blocks",               // Various
	"logs matched by query exceeds", // Various
	"query timeout exceeded",        // Timeout due to large range
	"read limit exceeded",           // Various
	"exceed maximum block range",    // Various
	"too much data",                 // Generic
	"result too large",              // Generic
}
