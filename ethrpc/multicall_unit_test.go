package ethrpc

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc/multicall"
)

type failingHTTPClient struct {
	err error
}

func (c failingHTTPClient) Do(*http.Request) (*http.Response, error) {
	return nil, c.err
}

func TestBatchCallPreservesTransportError(t *testing.T) {
	transportErr := errors.New("transport unavailable")
	provider, err := NewProvider(
		"http://example.invalid",
		WithHTTPClient(failingHTTPClient{err: transportErr}),
	)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	_, _, err = provider.BatchCall(context.Background(), []multicall.Call{{}})
	if !errors.Is(err, transportErr) {
		t.Fatalf("BatchCall() error = %v, want it to wrap %v", err, transportErr)
	}
}
