package ethrpc

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/goware/breaker"
)

type Option func(*Provider)

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func WithStreaming(nodeWebsocketURL string) Option {
	return func(p *Provider) {
		nodeWSURL := nodeWebsocketURL
		nodeWSURL = strings.Replace(nodeWSURL, "http://", "ws://", 1)
		nodeWSURL = strings.Replace(nodeWSURL, "https://", "wss://", 1)
		p.nodeWSURL = nodeWSURL
	}
}

func WithHTTPClient(c httpClient) Option {
	return func(p *Provider) {
		p.httpClient = c
	}
}

func WithLogger(log *slog.Logger) Option {
	return func(p *Provider) {
		p.log = log
	}
}

func WithBreaker(br breaker.Breaker) Option {
	return func(p *Provider) {
		p.br = br
	}
}

// func WithCache(cache cachestore.Store[[]byte]) Option {
// 	return func(p *Provider) {
// 		p.cache = cache
// 	}
// }

// 0: disabled, no validation (default)
// 1: semi-strict transactions – validates only transaction V, R, S values
// 2: strict block and transactions – validates block hash, sender address, and transaction signatures
func WithStrictness(strictness StrictnessLevel) Option {
	return func(p *Provider) {
		p.strictness = strictness
	}
}

func WithSemiValidation() Option {
	return func(p *Provider) {
		p.strictness = StrictnessLevel_Semi
	}
}

func WithStrictValidation() Option {
	return func(p *Provider) {
		p.strictness = StrictnessLevel_Strict
	}
}
