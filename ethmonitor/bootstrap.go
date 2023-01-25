package ethmonitor

import (
	"encoding/json"
	"fmt"

	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// BootstrapFromBlocks will bootstrap the ethmonitor canonical chain from input blocks,
// while also verifying the chain hashes link together.
func (c *Chain) BootstrapFromBlocks(blocks []*Block) error {
	return c.bootstrapBlocks(blocks)
}

// BootstrapFromBlocksJSON is convenience method which accepts json and bootstraps
// the ethmonitor chain. This method is here mostly for debugging purposes and recommend
// that you use BootstrapFromBlocks and handle constructing block events outside of ethmonitor.
func (c *Chain) BootstrapFromBlocksJSON(data []byte) error {
	var blocks Blocks
	err := json.Unmarshal(data, &blocks)
	if err != nil {
		return fmt.Errorf("ethmonitor: BootstrapFromBlocksJSON failed to unmarshal: %w", err)
	}
	return c.bootstrapBlocks(blocks)
}

func (c *Chain) bootstrapBlocks(blocks Blocks) error {
	if !c.bootstrapMode {
		return fmt.Errorf("ethmonitor: monitor must be in Bootstrap mode to use bootstrap methods")
	}
	if c.blocks != nil {
		return fmt.Errorf("ethmonitor: chain has already been bootstrapped")
	}

	if len(blocks) == 0 {
		c.blocks = make(Blocks, 0, c.retentionLimit)
		return nil
	}

	if len(blocks) == 1 {
		c.blocks = blocks.Copy()
		return nil
	}

	c.blocks = make(Blocks, 0, c.retentionLimit)

	if len(blocks) > c.retentionLimit {
		blocks = blocks[len(blocks)-c.retentionLimit-1:]
	}

	for _, b := range blocks {
		if b.Event == Added {
			err := c.push(b)
			if err != nil {
				return fmt.Errorf("ethmonitor: bootstrap failed to build canonical chain: %w", err)
			}
		} else {
			c.pop()
		}
	}

	return nil
}

func (c *Chain) Snapshot() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(c.blocks)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type blockSnapshot struct {
	Block *types.Block `json:"block"`
	Event Event        `json:"event"`
	Logs  []types.Log  `json:"logs"`
	OK    bool         `json:"ok"`
}

func (b *Block) MarshalJSON() ([]byte, error) {
	return json.Marshal(&blockSnapshot{
		Block: b.Block,
		Event: b.Event,
		Logs:  b.Logs,
		OK:    b.OK,
	})
}

func (b *Block) UnmarshalJSON(data []byte) error {
	var s *blockSnapshot
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	b.Block = s.Block
	b.Event = s.Event
	b.Logs = s.Logs
	b.OK = s.OK
	return nil
}
