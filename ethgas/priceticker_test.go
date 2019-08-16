package ethgas

import (
	"context"
	"github.com/go-test/deep"
	"math/big"
	"testing"
	"time"
)

type mockClient struct {
	prices []*big.Int
	pos    int
	err    error
}

func (c *mockClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	if c.err != nil {
		return nil, c.err
	}
	price := c.prices[c.pos]
	c.pos = (c.pos + 1) % len(c.prices)
	return price, nil
}

func TestPriceTicker(t *testing.T) {
	client := &mockClient{
		prices: []*big.Int{
			big.NewInt(1),
			big.NewInt(2),
			big.NewInt(3),
		},
	}
	errc := make(chan error)
	ticker := NewPriceTicker(time.Microsecond, PriceTickerParams{
		JSONRPC: client,
		Errc:    errc,
	})
	var prices []*big.Int
	for i := 0; i < 3; i++ {
		select {
		case price := <-ticker.P:
			if price.Time.IsZero() {
				t.Errorf("expected populated time field")
			}
			prices = append(prices, price.Wei)
		case err := <-errc:
			t.Fatal(err)
		}
	}
	ticker.Stop()
	if diff := deep.Equal(client.prices, prices); diff != nil {
		t.Error(diff)
	}
}

func TestPriceTickerTimeout(t *testing.T) {
	client := &mockClient{
		err: context.DeadlineExceeded,
	}
	errc := make(chan error)
	ticker := NewPriceTicker(time.Microsecond, PriceTickerParams{
		JSONRPC: client,
		Errc:    errc,
	})
	select {
	case <-ticker.P:
		t.Fatal("expected error on errc channel")
	case <-errc:
		ticker.Stop()
	}
}
