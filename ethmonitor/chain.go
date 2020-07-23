package ethmonitor

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	ErrUnexpectedParentHash  = fmt.Errorf("unexpected parent hash")
	ErrUnexpectedBlockNumber = fmt.Errorf("unexpected block number")
)

type Chain struct {
	blocks         []*Block
	retentionLimit int
	mu             sync.Mutex
}

func newChain(retentionLimit int) *Chain {
	return &Chain{
		blocks:         make([]*Block, 0, retentionLimit),
		retentionLimit: retentionLimit,
	}
}

// Push to the top of the stack
func (c *Chain) push(nextBlock *Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// New block validations
	n := len(c.blocks)
	if n > 0 {
		headBlock := c.blocks[n-1]

		// Pointing at prev block
		if nextBlock.ParentHash() != headBlock.Hash() {
			return ErrUnexpectedParentHash
		}

		// Block numbers are in sequence
		if nextBlock.NumberU64() != headBlock.NumberU64()+1 {
			return ErrUnexpectedBlockNumber
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

	n := len(c.blocks) - 1
	block := c.blocks[n]
	c.blocks[n] = nil
	c.blocks = c.blocks[:n]

	return block
}

func (c *Chain) Head() *Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.blocks) == 0 {
		return nil
	}
	return c.blocks[len(c.blocks)-1]
}

func (c *Chain) Blocks() []*Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocks
}

func (c *Chain) GetBlock(hash common.Hash) *Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.blocks) - 1; i >= 0; i-- {
		if c.blocks[i].Hash() == hash {
			return c.blocks[i]
		}
	}
	return nil
}

func (c *Chain) GetTransaction(hash common.Hash) *types.Transaction {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.blocks) - 1; i >= 0; i-- {
		for _, txn := range c.blocks[i].Transactions() {
			if txn.Hash() == hash {
				return txn
			}
		}
	}
	return nil
}

func (c *Chain) PrintAllBlocks() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, b := range c.blocks {
		fmt.Printf("<- [%d] %s\n", b.NumberU64(), b.Hash().Hex())
	}
}
