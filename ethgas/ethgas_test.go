package ethgas_test

import (
	"context"
	"testing"
	"time"

	"github.com/arcadeum/ethkit/ethgas"
	"github.com/arcadeum/ethkit/ethmonitor"
	"github.com/arcadeum/ethkit/ethrpc"
	"github.com/arcadeum/ethkit/util"
	"github.com/go-chi/httpvcr"
	"github.com/stretchr/testify/assert"
)

func TestGasTracker(t *testing.T) {
	testConfig := util.ReadTestFile(t)
	ethNodeURL := testConfig["INFURA_MAINNET_URL"]
	if ethNodeURL == "" {
		ethNodeURL = "http://localhost:8545"
	}

	ctx := context.Background()

	vcr := httpvcr.New("ethgas_mainnet")
	vcr.Start(ctx)
	defer vcr.Stop()

	vcr.URLRewriter = func(url string) string {
		// rewrite the url to hide the API keys
		return "http://mainnet/"
	}

	monitorOptions := ethmonitor.DefaultOptions
	if vcr.Mode() == httpvcr.ModeReplay {
		// change options to run replay tests faster
		monitorOptions.PollingInterval = 100 * time.Millisecond
	}

	// Setup provider and monitor
	provider, err := ethrpc.NewProvider(ethNodeURL)
	assert.NoError(t, err)

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	err = monitor.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer monitor.Stop()

	// Setup gas tracker
	gasTracker, err := ethgas.NewGasTracker(nil, monitor)
	assert.NoError(t, err)

	err = gasTracker.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer gasTracker.Stop()

	sub := gasTracker.Subscribe()
	defer sub.Unsubscribe()

	// Wait for requests to complete
	select {
	case <-vcr.Done():
		break
	case <-time.After(1 * time.Minute): // max amount of time to run the vcr recorder
		break
	}

	gasTracker.Stop()
	monitor.Stop()

	// assertions
	suggestedGasPrice := gasTracker.SuggestedGasPrice()
	assert.Equal(t, uint64(365), suggestedGasPrice.Fast)
	assert.Equal(t, uint64(340), suggestedGasPrice.Normal)
	assert.Equal(t, uint64(312), suggestedGasPrice.Slow)
	assert.Equal(t, uint64(10785102), suggestedGasPrice.BlockNum.Uint64())
	assert.Equal(t, uint64(1599093441), suggestedGasPrice.BlockTime)
}
