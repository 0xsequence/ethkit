package ethrpc

import (
	"testing"

	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/stretchr/testify/require"
)

func TestBatchCallUnmarshalJSON_OutOfOrder(t *testing.T) {
	batch := BatchCall{
		&Call{request: jsonrpc.NewRequest(1, "foo", nil)},
		&Call{request: jsonrpc.NewRequest(2, "bar", nil)},
	}

	data := []byte(`[
		{"jsonrpc":"2.0","id":2,"result":"0x02"},
		{"jsonrpc":"2.0","id":1,"result":"0x01"}
	]`)

	err := batch.UnmarshalJSON(data)
	require.NoError(t, err)
	require.NotNil(t, batch[0].response)
	require.NotNil(t, batch[1].response)
	require.Equal(t, uint64(1), batch[0].response.ID)
	require.Equal(t, uint64(2), batch[1].response.ID)
}

func TestBatchCallUnmarshalJSON_UnknownID(t *testing.T) {
	batch := BatchCall{
		&Call{request: jsonrpc.NewRequest(1, "foo", nil)},
	}

	err := batch.UnmarshalJSON([]byte(`{"jsonrpc":"2.0","id":2,"result":"0x02"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match any request")
}
