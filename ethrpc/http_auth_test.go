package ethrpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithHTTPAuthSetsRequestHeader(t *testing.T) {
	authHeaders := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders <- r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(chainIDResponse(body))
	}))
	defer srv.Close()

	p, err := ethrpc.NewProvider(srv.URL, ethrpc.WithHTTPAuth(func(h http.Header) error {
		h.Set("Authorization", "Bearer http-token")
		return nil
	}))
	require.NoError(t, err)

	chainID, err := p.ChainID(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1337), chainID.Int64())
	assert.Equal(t, "Bearer http-token", recvAuthHeader(t, authHeaders))
}

func TestWithHTTPAuthErrorAbortsRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	authErr := errors.New("token expired")
	p, err := ethrpc.NewProvider(srv.URL, ethrpc.WithHTTPAuth(func(h http.Header) error {
		return authErr
	}))
	require.NoError(t, err)

	_, err = p.ChainID(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, authErr)
	assert.ErrorIs(t, err, ethrpc.ErrRequestFail)
	assert.Equal(t, int32(0), hits.Load())
}

func TestWithHTTPAuthSetsWebSocketHandshakeHeader(t *testing.T) {
	authHeaders := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders <- r.Header.Get("Authorization")
		http.Error(w, "websocket upgrade not served in test", http.StatusBadRequest)
	}))
	defer srv.Close()

	p, err := ethrpc.NewProvider(srv.URL,
		ethrpc.WithStreaming(srv.URL),
		ethrpc.WithHTTPAuth(func(h http.Header) error {
			h.Set("Authorization", "Bearer ws-token")
			return nil
		}),
	)
	require.NoError(t, err)

	_, err = p.SubscribeNewHeads(context.Background(), make(chan *types.Header))
	require.Error(t, err)
	assert.Equal(t, "Bearer ws-token", recvAuthHeader(t, authHeaders))
}

func TestWithHTTPAuthErrorAbortsWebSocketDial(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "websocket upgrade not served in test", http.StatusBadRequest)
	}))
	defer srv.Close()

	authErr := errors.New("token expired")
	p, err := ethrpc.NewProvider(srv.URL,
		ethrpc.WithStreaming(srv.URL),
		ethrpc.WithHTTPAuth(func(h http.Header) error {
			return authErr
		}),
	)
	require.NoError(t, err)

	_, err = p.SubscribeNewHeads(context.Background(), make(chan *types.Header))
	require.Error(t, err)
	assert.ErrorIs(t, err, authErr)
	assert.Equal(t, int32(0), hits.Load())
}

func recvAuthHeader(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case h := <-ch:
		return h
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for request at httptest server")
		return ""
	}
}

func chainIDResponse(body []byte) []byte {
	type response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  string          `json:"result"`
	}

	var single struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(body, &single); err == nil {
		out, _ := json.Marshal(response{JSONRPC: "2.0", ID: single.ID, Result: "0x539"})
		return out
	}

	var batch []struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(body, &batch); err != nil {
		return nil
	}
	out := make([]response, 0, len(batch))
	for _, c := range batch {
		out = append(out, response{JSONRPC: "2.0", ID: c.ID, Result: "0x539"})
	}
	res, _ := json.Marshal(out)
	return res
}
