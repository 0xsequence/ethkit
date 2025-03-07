package ethproviders_test

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"testing"

	"github.com/0xsequence/ethkit/ethproviders"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/go-chi/traceid"
	"github.com/go-chi/transport"
	"github.com/stretchr/testify/require"
)

func TestBasic(t *testing.T) {
	cfg := ethproviders.Config{
		"polygon": ethproviders.NetworkConfig{
			ID:  137,
			URL: "https://dev-nodes.sequence.app/polygon",
		},
	}

	ps, err := ethproviders.NewProviders(cfg)
	require.NoError(t, err)
	p := ps.Get("polygon")
	require.NotNil(t, p)

	block, err := p.BlockByNumber(context.Background(), big.NewInt(1_000_000))
	require.NoError(t, err)
	require.NotNil(t, block)
	require.Equal(t, uint64(1_000_000), block.NumberU64())
}

func TestClientWithJWTAuth(t *testing.T) {
	// NODE_URL="https://dev-nodes.sequence.app"
	// JWT_TOKEN=$(jwtutil -secret=changemenow -encode -claims='{"service":"test"}' 2>/dev/null)

	nodeURL := os.Getenv("NODE_URL")
	jwtToken := os.Getenv("JWT_TOKEN")

	if jwtToken == "" || nodeURL == "" {
		t.Skip("NODE_URL or JWT_TOKEN is not set")
	}

	cfg := ethproviders.Config{
		"polygon": ethproviders.NetworkConfig{
			ID:  137,
			URL: fmt.Sprintf("%s/polygon", nodeURL),
		},
	}

	httpClient := &http.Client{
		Transport: transport.Chain(http.DefaultTransport,
			traceid.Transport,
			transport.SetHeaderFunc("Authorization", func(req *http.Request) string {
				return "BEARER " + jwtToken
			}),
			transport.LogRequests(transport.LogOptions{Concise: true, CURL: true}),
		),
	}

	ps, err := ethproviders.NewProviders(cfg, ethrpc.WithHTTPClient(httpClient))
	require.NoError(t, err)
	p := ps.Get("polygon")
	require.NotNil(t, p)

	block, err := p.BlockByNumber(context.Background(), big.NewInt(1_000_000))
	require.NoError(t, err)
	require.NotNil(t, block)
	require.Equal(t, uint64(1_000_000), block.NumberU64())
}
