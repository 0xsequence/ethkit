package ethmonitor

import "github.com/ethereum/go-ethereum/core/types"

type BlockType uint32

const (
	Added = iota
	Removed
)

type Block struct {
	*types.Block
	Type BlockType
	Logs []types.Log
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

type Subscription interface {
	Blocks() <-chan Blocks
	Done() <-chan struct{}
	Unsubscribe() func()
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

func (s *subscriber) Unsubscribe() func() {
	return s.unsubscribe
}
