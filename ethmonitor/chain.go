package ethmonitor

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
)

var (
	ErrUnexpectedParentHash  = errors.New("unexpected parent hash")
	ErrUnexpectedBlockNumber = errors.New("unexpected block number")
)

const NumCanonicalBlocks = 20

type Chain struct {
	blocks []*types.Block // up to `NumCanonicalBlocks`

	mu sync.Mutex
}

func newChain() *Chain {
	return &Chain{
		blocks: make([]*types.Block, 0, NumCanonicalBlocks),
	}
}

// Push to the top of the stack
func (c *Chain) push(nextBlock *types.Block) error {
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
	if len(c.blocks) > NumCanonicalBlocks {
		c.blocks[0] = nil
		c.blocks = c.blocks[1:]
	}

	return nil
}

// Pop from the top of the stack
func (c *Chain) pop() *types.Block {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := len(c.blocks) - 1
	block := c.blocks[n]
	c.blocks[n] = nil
	c.blocks = c.blocks[:n]

	return block
}

func (c *Chain) Head() *types.Block {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.blocks) == 0 {
		return nil
	}

	return c.blocks[len(c.blocks)-1]
}

func (c *Chain) Blocks() []*types.Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocks
}

func (c *Chain) FindBlockHash(hash common.Hash) *types.Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.blocks) - 1; i >= 0; i-- {
		if c.blocks[i].Hash() == hash {
			return c.blocks[i]
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
