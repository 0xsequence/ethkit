package ethrpc

import (
	"net/http"

	"github.com/goware/breaker"
	"github.com/goware/logger"
)

type Option func(*Provider)

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func WithHTTPClient(c httpClient) Option {
	return func(p *Provider) {
		p.httpClient = c
	}
}

func WithLogger(log logger.Logger) Option {
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

func WithJWTAuthorization(jwtToken string) Option {
	return func(p *Provider) {
		p.jwtToken = jwtToken
	}
}
