package ethmonitor

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type BlockType uint32

const (
	Added BlockType = iota
	Removed
	Updated
)

type Block struct {
	*types.Block
	Type          BlockType
	Logs          []types.Log
	getLogsFailed bool
}

type Blocks []*Block

func (b Blocks) LatestBlock() *Block {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i].Type == Added {
			return b[i]
		}
	}
	return nil
}

func (b Blocks) FindBlock(hash common.Hash) (*Block, bool) {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i].Hash() == hash {
			return b[i], true
		}
	}
	return nil, false
}

type Subscription interface {
	Blocks() <-chan Blocks
	Done() <-chan struct{}
	Unsubscribe()
}

type subscriber struct {
	ch          chan Blocks
	done        chan struct{}
	unsubscribe func()
}

func (s *subscriber) Blocks() <-chan Blocks {
	return s.ch
}

func (s *subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *subscriber) Unsubscribe() {
	s.unsubscribe()
}
