package ethmonitor

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func canonicalTestBlock(num int64) *Block {
	return &Block{
		Block: types.NewBlockWithHeader(&types.Header{
			Number:     big.NewInt(num),
			BlockHash:  common.BigToHash(big.NewInt(num)),
			ParentHash: common.BigToHash(big.NewInt(num - 1)),
			Time:       uint64(num),
		}),
		Event: Added,
		OK:    true,
	}
}

func pushCanonicalTestBlock(t *testing.T, chain *Chain, input *Block) *Block {
	t.Helper()
	chain.push(input)
	block := chain.Head()
	require.NotNil(t, block)
	require.Equal(t, input.Hash(), block.Hash())
	return block
}

// The interface keeps the before-fix proof executable without the new method.
func canonicalTestState(t *testing.T, block *Block) (uint64, bool) {
	t.Helper()
	state, ok := any(block).(interface{ CanonicalState() (uint64, bool) })
	if !ok {
		return 0, false
	}
	return state.CanonicalState()
}

func TestBlockCanonicalStateRetention(t *testing.T) {
	for _, depth := range []int64{1, 3} {
		t.Run(fmt.Sprintf("evictionDepth=%d", depth), func(t *testing.T) {
			chain := newChain(10, false)
			block := pushCanonicalTestBlock(t, chain, canonicalTestBlock(100))
			copy := chain.Blocks().Copy()[0]
			incarnation, canonical := canonicalTestState(t, block)
			require.Positive(t, incarnation)
			require.True(t, canonical)
			for num := int64(101); num <= 109+depth; num++ {
				pushCanonicalTestBlock(t, chain, canonicalTestBlock(num))
			}
			require.Nil(t, chain.GetBlock(block.Hash()))
			for _, snapshot := range []*Block{block, copy} {
				got, canonical := canonicalTestState(t, snapshot)
				require.Equal(t, incarnation, got)
				require.True(t, canonical, "retention eviction was mistaken for removal")
			}
		})
	}
}

func TestBlockCanonicalStateReadoption(t *testing.T) {
	chain := newChain(10, false)
	input := canonicalTestBlock(100)
	block := pushCanonicalTestBlock(t, chain, input)
	shallow := *block
	copy := chain.Blocks().Copy()[0]
	incarnation, canonical := canonicalTestState(t, block)
	require.Positive(t, incarnation)
	require.True(t, canonical)
	removed := *chain.pop()
	removed.Event = Removed
	for _, snapshot := range []*Block{block, &shallow, copy, &removed} {
		got, canonical := canonicalTestState(t, snapshot)
		require.Equal(t, incarnation, got)
		require.False(t, canonical, "snapshot did not observe the actual removal")
	}

	// Reusing either the original input or a removed snapshot must create a fresh
	// owned incarnation without mutating old Added/Removed event copies.
	for _, reused := range []*Block{input, copy} {
		fresh := pushCanonicalTestBlock(t, chain, reused)
		got, canonical := canonicalTestState(t, fresh)
		require.Greater(t, got, incarnation)
		require.True(t, canonical)
		for _, snapshot := range []*Block{block, &shallow, copy, &removed} {
			got, canonical := canonicalTestState(t, snapshot)
			require.Equal(t, incarnation, got)
			require.False(t, canonical, "fresh readoption revived an old event")
		}
		chain.pop()
	}
}

func TestBlockCanonicalStateBootstrap(t *testing.T) {
	for _, count := range []int{1, 3} {
		for _, serialized := range []bool{false, true} {
			t.Run(fmt.Sprintf("blocks=%d/JSON=%v", count, serialized), func(t *testing.T) {
				inputs := make(Blocks, count)
				for i := range inputs {
					inputs[i] = canonicalTestBlock(int64(100 + i))
				}
				chain := newChain(10, true)
				if serialized {
					data, err := json.Marshal(inputs)
					require.NoError(t, err)
					require.NoError(t, chain.BootstrapFromBlocksJSON(data))
				} else {
					require.NoError(t, chain.BootstrapFromBlocks(inputs))
				}
				for _, block := range chain.Blocks().Copy() {
					incarnation, canonical := canonicalTestState(t, block)
					require.Positive(t, incarnation)
					require.True(t, canonical)
					data, err := json.Marshal(block)
					require.NoError(t, err)
					require.NotContains(t, string(data), "incarnation")
					require.NoError(t, json.Unmarshal(data, block))
					incarnation, canonical = canonicalTestState(t, block)
					require.Zero(t, incarnation, "serialized state preserved runtime ownership")
					require.False(t, canonical)
				}
			})
		}
	}
}

func TestBlockCanonicalStateConcurrentRemoval(t *testing.T) {
	chain := newChain(10, false)
	input := canonicalTestBlock(100)
	block := pushCanonicalTestBlock(t, chain, input)
	incarnation, _ := canonicalTestState(t, block)
	require.Positive(t, incarnation)
	state := any(block).(interface{ CanonicalState() (uint64, bool) })
	start, done := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(start)
		removed := false
		for {
			got, canonical := state.CanonicalState()
			if got != incarnation || (removed && canonical) {
				result <- fmt.Errorf("old incarnation changed or revived: id=%d canonical=%v", got, canonical)
				return
			}
			removed = removed || !canonical
			select {
			case <-done:
				result <- nil
				return
			default:
			}
		}
	}()
	<-start
	for i := 0; i < 100; i++ {
		chain.pop()
		pushCanonicalTestBlock(t, chain, input)
	}
	close(done)
	require.NoError(t, <-result)
	got, canonical := state.CanonicalState()
	require.Equal(t, incarnation, got)
	require.False(t, canonical)
}

func TestBlockCanonicalStateUntracked(t *testing.T) {
	for _, block := range []*Block{nil, canonicalTestBlock(100)} {
		incarnation, canonical := canonicalTestState(t, block)
		require.Zero(t, incarnation)
		require.False(t, canonical)
	}
}
