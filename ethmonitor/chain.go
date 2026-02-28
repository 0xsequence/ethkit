package ethmonitor

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sync"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/params"
	"github.com/0xsequence/ethkit/go-ethereum/rpc"
)

type Chain struct {
	// blocks ordered from oldest to newest
	blocks Blocks

	// retentionLimit of total number of blocks in cache
	retentionLimit int

	// bootstrapMode flag that chain is bootstrapped with blocks
	// before starting the monitor.
	bootstrapMode bool

	mu               sync.RWMutex
	averageBlockTime float64 // in seconds
}

func newChain(retentionLimit int, bootstrapMode bool) *Chain {
	// a minimum retention limit
	retentionMin := 10
	if retentionLimit < retentionMin {
		retentionLimit = retentionMin
	}

	// blocks of nil means the chain has not been initialized
	var blocks Blocks = nil
	if !bootstrapMode {
		blocks = make(Blocks, 0, retentionLimit)
	}

	return &Chain{
		blocks:         blocks,
		retentionLimit: retentionLimit,
		bootstrapMode:  bootstrapMode,
	}
}

// TODO: unused method..
// func (c *Chain) clear() {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
// 	c.blocks = c.blocks[:0]
// 	c.averageBlockTime = 0
// }

// Push to the top of the stack
func (c *Chain) push(nextBlock *Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// New block validations
	n := len(c.blocks)
	if n > 0 {
		headBlock := c.blocks[n-1]

		// Assert pointing at prev block
		if nextBlock.ParentHash() != headBlock.Hash() {
			return ErrUnexpectedParentHash
		}

		// Assert block numbers are in sequence
		if nextBlock.NumberU64() != headBlock.NumberU64()+1 {
			return ErrUnexpectedBlockNumber
		}

		// Update average block time
		if c.averageBlockTime == 0 {
			c.averageBlockTime = float64(nextBlock.Time() - headBlock.Time())
		} else {
			c.averageBlockTime = (c.averageBlockTime + float64(nextBlock.Time()-headBlock.Time())) / 2
		}
	}

	// Add to head of stack
	c.blocks = append(c.blocks, nextBlock)
	if len(c.blocks) > c.retentionLimit {
		c.blocks[0] = nil
		c.blocks = c.blocks[1:]
	}

	return nil
}

// Pop from the top of the stack
func (c *Chain) pop() *Block {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.blocks) == 0 {
		return nil
	}

	n := len(c.blocks) - 1
	block := c.blocks[n]
	c.blocks[n] = nil
	c.blocks = c.blocks[:n]
	return block
}

func (c *Chain) Head() *Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocks.Head()
}

func (c *Chain) Tail() *Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocks.Tail()
}

func (c *Chain) Blocks() Blocks {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Copy only OK blocks
	last := len(c.blocks) - 1
	for i := last; i >= 0; i-- {
		if c.blocks[i].OK {
			break
		}
		last = i
	}
	last += 1

	blocks := make(Blocks, last)
	copy(blocks, c.blocks[:last])

	return blocks
}

func (c *Chain) ReadyHead() *Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.blocks) - 1; i >= 0; i-- {
		if c.blocks[i].OK {
			return c.blocks[i]
		}
	}
	return nil
}

func (c *Chain) GetBlock(hash common.Hash) *Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	block, _ := c.blocks.FindBlock(hash)
	return block
}

func (c *Chain) GetBlockByNumber(blockNum uint64, event Event) *Block {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := len(c.blocks) - 1; i >= 0; i-- {
		if c.blocks[i].NumberU64() == blockNum && c.blocks[i].Event == event {
			return c.blocks[i]
		}
	}
	return nil
}

// GetTransaction searches our canonical chain of blocks (where each block points at previous),
// and returns the transaction. Aka, searches our chain for mined transactions. Keep in mind
// transactions can still be reorged, but you can check the blockNumber and compare it against
// the head to determine if its final.
func (c *Chain) GetTransaction(txnHash common.Hash) (*types.Transaction, Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find any transaction added or removed in the retention cache
	for i := len(c.blocks) - 1; i >= 0; i-- {
		for _, txn := range c.blocks[i].Transactions() {
			if txn.Hash() == txnHash {
				return txn, c.blocks[i].Event
			}
		}
	}

	return nil, 0
}

func (c *Chain) PrintAllBlocks() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, b := range c.blocks {
		fmt.Printf("<- [%d] %s\n", b.NumberU64(), b.Hash().Hex())
	}
}

func (c *Chain) GetAverageBlockTime() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.averageBlockTime
}

type Event uint32

const (
	Added Event = iota
	Removed
)

type Block struct {
	*types.Block

	// Event type where Block is Added or Removed (ie. reorged)
	Event Event

	// Logs in the block, grouped by transactions:
	// [[txnA logs, ..], [txnB logs, ..], ..]
	// Logs [][]types.Log `json:"logs"`
	Logs []types.Log

	// OK flag which represents the block is ready for broadcasting
	OK bool
}

type Blocks []*Block

const feeHistoryMaxQueryLimit = 101

var (
	errInvalidRewardPercentile = errors.New("invalid reward percentile")
	errRequestBeyondHeadBlock  = errors.New("request beyond head block")
)

// FeeHistory approximates the behaviour of eth_feeHistory.
// It intentionally deviates from standard to avoid fetching transaction receipts.
// It uses transaction gas limits instead of actual gas used, and transaction priority fees instead of effective priority fees.
func (b Blocks) FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	if blockCount < 1 {
		return &ethereum.FeeHistory{OldestBlock: new(big.Int)}, nil
	}

	if len(rewardPercentiles) > feeHistoryMaxQueryLimit {
		return nil, fmt.Errorf("%w: over the query limit %d", errInvalidRewardPercentile, feeHistoryMaxQueryLimit)
	}
	for i, p := range rewardPercentiles {
		if p < 0 || p > 100 {
			return nil, fmt.Errorf("%w: %f", errInvalidRewardPercentile, p)
		}
		if i > 0 && p < rewardPercentiles[i-1] {
			return nil, fmt.Errorf("%w: #%d:%f >= #%d:%f", errInvalidRewardPercentile, i-1, rewardPercentiles[i-1], i, p)
		}
	}

	blocksByNumber := make(map[uint64]*Block, len(b))
	var (
		headNum uint64
		tailNum uint64
		have    bool
	)
	for _, block := range b {
		if block == nil || block.Block == nil || block.Event != Added {
			continue
		}
		num := block.NumberU64()
		blocksByNumber[num] = block
		if !have || num > headNum {
			headNum = num
		}
		if !have || num < tailNum {
			tailNum = num
		}
		have = true
	}
	if !have {
		return &ethereum.FeeHistory{OldestBlock: new(big.Int)}, nil
	}

	requestedBlocks := blockCount
	var lastNum uint64
	switch {
	case lastBlock == nil:
		lastNum = headNum
	case lastBlock.Sign() >= 0:
		if !lastBlock.IsUint64() {
			return nil, fmt.Errorf("%w: requested %s, head %d", errRequestBeyondHeadBlock, lastBlock.String(), headNum)
		}
		lastNum = lastBlock.Uint64()
	default:
		if !lastBlock.IsInt64() {
			return nil, fmt.Errorf("invalid block number: %s", lastBlock.String())
		}
		switch rpc.BlockNumber(lastBlock.Int64()) {
		case rpc.PendingBlockNumber:
			if requestedBlocks > 0 {
				requestedBlocks--
			}
			lastNum = headNum
		case rpc.LatestBlockNumber, rpc.SafeBlockNumber, rpc.FinalizedBlockNumber:
			lastNum = headNum
		case rpc.EarliestBlockNumber:
			lastNum = tailNum
		default:
			return nil, fmt.Errorf("invalid block number: %s", lastBlock.String())
		}
	}

	if requestedBlocks == 0 {
		return &ethereum.FeeHistory{OldestBlock: new(big.Int)}, nil
	}
	if lastNum > headNum {
		return nil, fmt.Errorf("%w: requested %d, head %d", errRequestBeyondHeadBlock, lastNum, headNum)
	}
	if lastNum < tailNum {
		return &ethereum.FeeHistory{OldestBlock: new(big.Int)}, nil
	}

	maxAvailable := lastNum - tailNum + 1
	if requestedBlocks > maxAvailable {
		requestedBlocks = maxAvailable
	}
	if requestedBlocks == 0 {
		return &ethereum.FeeHistory{OldestBlock: new(big.Int)}, nil
	}

	oldestNum := lastNum + 1 - requestedBlocks
	finalCount := requestedBlocks
	for i := uint64(0); i < requestedBlocks; i++ {
		if blocksByNumber[oldestNum+i] == nil {
			finalCount = i
			break
		}
	}
	if finalCount == 0 {
		return &ethereum.FeeHistory{OldestBlock: new(big.Int)}, nil
	}

	var reward [][]*big.Int
	if len(rewardPercentiles) != 0 {
		reward = make([][]*big.Int, finalCount)
	}
	baseFee := make([]*big.Int, finalCount+1)
	gasUsedRatio := make([]float64, finalCount)

	type txGasAndReward struct {
		gasLimit uint64
		reward   *big.Int
	}

	for i := uint64(0); i < finalCount; i++ {
		block := blocksByNumber[oldestNum+i]
		if block == nil {
			continue
		}

		if bf := block.BaseFee(); bf != nil {
			baseFee[i] = new(big.Int).Set(bf)
		} else {
			baseFee[i] = new(big.Int)
		}

		if gasLimit := block.GasLimit(); gasLimit > 0 {
			gasUsedRatio[i] = float64(block.GasUsed()) / float64(gasLimit)
		}

		if len(rewardPercentiles) == 0 {
			continue
		}

		txs := block.Transactions()
		rewards := make([]*big.Int, len(rewardPercentiles))
		if len(txs) == 0 {
			for j := range rewards {
				rewards[j] = new(big.Int)
			}
			reward[i] = rewards
			continue
		}

		sorter := make([]txGasAndReward, len(txs))
		var totalGasLimit uint64
		for j, tx := range txs {
			gasLimit := tx.Gas()
			totalGasLimit += gasLimit
			sorter[j] = txGasAndReward{
				gasLimit: gasLimit,
				reward:   tx.GasTipCap(),
			}
		}
		slices.SortStableFunc(sorter, func(a, b txGasAndReward) int {
			return a.reward.Cmp(b.reward)
		})

		var txIndex int
		cumulativeGasLimit := sorter[0].gasLimit

		for j, p := range rewardPercentiles {
			thresholdGasLimit := uint64(float64(totalGasLimit) * p / 100)
			for cumulativeGasLimit < thresholdGasLimit && txIndex < len(sorter)-1 {
				txIndex++
				cumulativeGasLimit += sorter[txIndex].gasLimit
			}
			rewards[j] = new(big.Int).Set(sorter[txIndex].reward)
		}
		reward[i] = rewards
	}

	lastInRange := oldestNum + finalCount - 1
	nextBlock := blocksByNumber[lastInRange+1]
	switch {
	case nextBlock != nil && nextBlock.BaseFee() != nil:
		baseFee[finalCount] = new(big.Int).Set(nextBlock.BaseFee())
	case blocksByNumber[lastInRange] != nil && blocksByNumber[lastInRange].BaseFee() != nil:
		baseFee[finalCount] = calcNextBaseFee(blocksByNumber[lastInRange])
	default:
		baseFee[finalCount] = new(big.Int)
	}

	if len(rewardPercentiles) == 0 {
		reward = nil
	}

	return &ethereum.FeeHistory{
		OldestBlock:  new(big.Int).SetUint64(oldestNum),
		Reward:       reward,
		BaseFee:      baseFee,
		GasUsedRatio: gasUsedRatio,
	}, nil
}

func calcNextBaseFee(parent *Block) *big.Int {
	if parent == nil || parent.Block == nil {
		return new(big.Int)
	}

	parentBaseFee := parent.BaseFee()
	if parentBaseFee == nil {
		return new(big.Int)
	}

	parentGasTarget := parent.GasLimit() / params.DefaultElasticityMultiplier
	if parentGasTarget == 0 {
		return new(big.Int).Set(parentBaseFee)
	}

	if parent.GasUsed() == parentGasTarget {
		return new(big.Int).Set(parentBaseFee)
	}

	num := new(big.Int)
	denom := new(big.Int)

	if parent.GasUsed() > parentGasTarget {
		num.SetUint64(parent.GasUsed() - parentGasTarget)
		num.Mul(num, parentBaseFee)
		num.Div(num, denom.SetUint64(parentGasTarget))
		num.Div(num, denom.SetUint64(params.DefaultBaseFeeChangeDenominator))
		if num.Sign() == 0 {
			num.SetUint64(1)
		}
		return num.Add(parentBaseFee, num)
	}

	num.SetUint64(parentGasTarget - parent.GasUsed())
	num.Mul(num, parentBaseFee)
	num.Div(num, denom.SetUint64(parentGasTarget))
	num.Div(num, denom.SetUint64(params.DefaultBaseFeeChangeDenominator))
	baseFee := num.Sub(parentBaseFee, num)
	if baseFee.Sign() < 0 {
		return new(big.Int)
	}
	return baseFee
}

func (b Blocks) LatestBlock() *Block {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i].Event == Added {
			return b[i]
		}
	}
	return nil
}

func (b Blocks) Head() *Block {
	if len(b) == 0 {
		return nil
	}
	return b[len(b)-1]
}

func (b Blocks) Tail() *Block {
	if len(b) == 0 {
		return nil
	}
	return b[0]
}

func (b Blocks) IsOK() bool {
	for _, block := range b {
		if !block.OK {
			return false
		}
	}
	return true
}

func (b Blocks) Reorg() bool {
	for _, block := range b {
		if block.Event == Removed {
			return true
		}
	}
	return false
}

func (blocks Blocks) FindBlock(blockHash common.Hash, optEvent ...Event) (*Block, bool) {
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Hash() == blockHash {
			if optEvent == nil {
				return blocks[i], true
			} else if len(optEvent) > 0 && blocks[i].Event == optEvent[0] {
				return blocks[i], true
			}
		}
	}
	return nil, false
}

func (blocks Blocks) EventExists(block *types.Block, event Event) bool {
	b, ok := blocks.FindBlock(block.Hash(), event)
	if !ok {
		return false
	}
	if b.ParentHash() == block.ParentHash() && b.NumberU64() == block.NumberU64() {
		return true
	}
	return false
}

func (blocks Blocks) Copy() Blocks {
	nb := make(Blocks, len(blocks))

	for i, b := range blocks {
		var logs []types.Log
		if b.Logs != nil {
			copy(logs, b.Logs)
		}

		nb[i] = &Block{
			Block: b.Block,
			Event: b.Event,
			Logs:  logs,
			OK:    b.OK,
		}
	}

	return nb
}

func IsBlockEq(a, b *types.Block) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Hash() == b.Hash() && a.NumberU64() == b.NumberU64() && a.ParentHash() == b.ParentHash()
}
