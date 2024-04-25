package ethcoder_test

import (
	"testing"

	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/stretchr/testify/require"
)

func TestEventTopicHash(t *testing.T) {
	in := []struct {
		event string
	}{
		{"Transfer(address indexed from, address indexed to, uint256 value)"},
		{"Transfer(address from, address indexed to, uint256 value)"},
		{"Transfer(address, address , uint256 )"},
	}

	for _, x := range in {
		topicHash, err := ethcoder.EventTopicHash(x.event)
		require.NoError(t, err)
		require.Equal(t, "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", topicHash.String())
	}
}
