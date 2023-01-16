package ethrpc2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync/atomic"

	"github.com/goware/breaker"
	"github.com/goware/cachestore"
	"github.com/goware/logger"
)

type Provider struct {
	log        logger.Logger
	nodeURL    string
	httpClient httpClient
	br         breaker.Breaker

	chainID *big.Int
	cache   cachestore.Store[[]byte]
	lastID  atomic.Uint32
}

func NewProvider(ctx context.Context, nodeURL string, options ...Option) (*Provider, error) {
	p := &Provider{
		nodeURL:    nodeURL,
		httpClient: http.DefaultClient,
	}

	for _, opt := range options {
		opt(p)
	}

	var err error
	p.chainID, err = p.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) Do(ctx context.Context, calls ...Call) error {
	if len(calls) == 0 {
		return nil
	}

	batchReq := make([]*jsonrpcMessage, 0, len(calls))
	for i, call := range calls {
		if call.err != nil {
			// TODO: store and return the error but execute the rest of the batch?
			return fmt.Errorf("call %d has an error: %w", i, call.err)
		}

		jrpcReq := call.request
		jrpcReq.ID = p.lastID.Add(1)
		batchReq = append(batchReq, jrpcReq)
	}

	var reqBody any = batchReq
	if len(batchReq) == 1 {
		reqBody = batchReq[0]
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal JSONRPC request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, p.nodeURL, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("failed to initialize http.Request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	res, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer res.Body.Close()

	var (
		results []jsonrpcMessage
		target  any = &results
	)
	if len(batchReq) == 1 {
		results = make([]jsonrpcMessage, 1)
		target = &results[0]
	}

	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	for i, result := range results {
		// TODO: handle batch errors
		if e := result.Error; e != nil {
			return fmt.Errorf("JSONRPC error %d: %s", e.Code, e.Message)
		}
		if err := calls[i].resultFn(result.Result); err != nil {
			return fmt.Errorf("failed to store result value: %w", err)
		}
	}

	return nil
}

var _ Interface = (*Provider)(nil)
