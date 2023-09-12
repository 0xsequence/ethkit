package ethmonitor

import (
	"fmt"
	"sync"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type Chain struct {
	// blocks ordered from oldest to newest
	blocks Blocks

	// retentionLimit of total number of blocks in cache
	retentionLimit int

	// bootstrapMode flag that chain is bootstrapped with blocks
	// before starting the monitor.
	bootstrapMode bool

	mu               sync.Mutex
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
	c.mu.Lock()
	defer c.mu.Unlock()
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

	// Raw byte payloads for block and logs responses from the nodes.
	// The values are only set if RetainPayloads is set to true on monitor.
	BlockPayload []byte
	LogsPayload  []byte
}

type Blocks []*Block

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

		var blockPayload []byte
		if b.BlockPayload != nil {
			copy(blockPayload, b.BlockPayload)
		}

		var logsPayload []byte
		if b.LogsPayload != nil {
			copy(logsPayload, b.LogsPayload)
		}

		nb[i] = &Block{
			Block:        b.Block,
			Event:        b.Event,
			Logs:         logs,
			OK:           b.OK,
			BlockPayload: blockPayload,
			LogsPayload:  logsPayload,
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
