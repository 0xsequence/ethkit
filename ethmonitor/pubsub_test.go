package ethmonitor

import (
	"math/big"
	"strings"
	"testing"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestQueueBasic(t *testing.T) {
	qu := newQueue(100)

	require.True(t, qu.len() == 0)

	// blocks := mockBlockchain(5)
	// for _, b := range blocks {
	// 	fmt.Println("=>", b.NumberU64(), b.Hash().Hex(), b.ParentHash().Hex())
	// }

	events := Blocks{}
	for i, b := range mockBlockchain(5) {
		_ = i
		events = append(events, &Block{
			Block: b,
			Event: Added,
			OK:    i < 3, // first 3 are OK
			// OK: true,
		})
	}

	err := qu.enqueue(events)
	require.NoError(t, err)
	require.Len(t, qu.events, 5)

	events2, ok := qu.dequeue(0)
	require.True(t, ok)
	require.NotEmpty(t, events2)
	require.Len(t, events2, 3)

	require.Equal(t, uint64(1), events[0].Block.NumberU64())
	require.Equal(t, uint64(2), events[1].Block.NumberU64())
	require.Equal(t, uint64(3), events[2].Block.NumberU64())
}

func TestQueueMore(t *testing.T) {
	qu := newQueue(100)

	require.True(t, qu.len() == 0)

	blocks := mockBlockchain(10)

	// TODO: we can add more tests to join/merge blocks as it relates
	// to the `chain#sweep` method.

	events := Blocks{
		{Block: blocks[0], Event: Added, OK: true},
	}

	err := qu.enqueue(events)
	require.NoError(t, err)
	require.Len(t, qu.events, 1)

	events2, ok := qu.dequeue(0)
	require.True(t, ok)
	require.NotEmpty(t, events2)
	require.Len(t, events2, 1)

	require.Equal(t, uint64(1), events[0].Block.NumberU64())
}

func mockBlockchain(size int) []*types.Block {
	bc := []*types.Block{}
	for i := 0; i < size; i++ {
		var parentHash string
		if i == 0 {
			parentHash = "0x0"
		} else {
			parentHash = bc[i-1].Hash().Hex()
		}
		b := mockBlock(parentHash, i+1)
		bc = append(bc, b)
	}
	return bc
}

func mockBlock(parentHash string, blockNum int) *types.Block {
	if !strings.HasPrefix(parentHash, "0x") {
		panic("parentHash needs 0x prefix")
	}
	return types.NewBlockWithHeader(&types.Header{
		ParentHash: common.HexToHash(parentHash),
		Number:     big.NewInt(int64(blockNum)),
	})
}
