package ethgas

import (
	"context"
	"math/big"
	"time"
)

type Price struct {
	Wei  *big.Int
	Time time.Time
}

type PriceTickerParams struct {
	JSONRPC JSONRPC
	Timeout time.Duration
	Errc    chan<- error
}

type JSONRPC interface {
	SuggestGasPrice(context.Context) (*big.Int, error)
}

func NewPriceTicker(interval time.Duration, params PriceTickerParams) *PriceTicker {
	prices := make(chan Price)
	ticker := time.NewTicker(interval)
	pt := &PriceTicker{
		P:       prices,
		ticker:  ticker,
		jsonrpc: params.JSONRPC,
		timeout: params.Timeout,
		errc:    params.Errc,
	}
	go func() {
		for now := range ticker.C {
			pt.tick(now, prices)
		}
		close(prices)
	}()
	return pt
}

type PriceTicker struct {
	P       <-chan Price
	ticker  *time.Ticker
	jsonrpc JSONRPC
	timeout time.Duration
	errc    chan<- error
}

func (pt *PriceTicker) tick(now time.Time, prices chan<- Price) {
	ctx := context.Background()
	if pt.timeout > 0 {
		ctx, _ = context.WithTimeout(ctx, pt.timeout)
	}
	wei, err := pt.jsonrpc.SuggestGasPrice(ctx)
	if err != nil {
		// Avoid blocking on error channel
		select {
		case pt.errc <- err:
		default:
		}
		return
	}
	price := Price{Wei: wei, Time: now}
	prices <- price
}

func (pt *PriceTicker) Stop() {
	pt.ticker.Stop()
}
