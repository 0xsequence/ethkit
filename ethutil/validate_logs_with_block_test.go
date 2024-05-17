package ethutil

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/stretchr/testify/require"
)

func TestValidateLogsWithBlockHeader(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	header, err := p.HeaderByNumber(context.Background(), big.NewInt(20_000_003))
	require.NoError(t, err)
	require.NotNil(t, header)

	logs, err := p.FilterLogs(context.Background(), ethereum.FilterQuery{
		FromBlock: big.NewInt(20_000_003),
		ToBlock:   big.NewInt(20_000_003),
	})
	require.NoError(t, err)

	require.True(t, ValidateLogsWithBlockHeader(logs, header))
}
