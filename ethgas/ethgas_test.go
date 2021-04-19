package ethgas_test

import (
	"context"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethgas"
	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/util"
	"github.com/go-chi/httpvcr"
	"github.com/stretchr/testify/assert"
)

func TestGasGauge(t *testing.T) {
	testConfig, err := util.ReadTestConfig("../ethkit-test.json")
	if err != nil {
		t.Error(err)
	}

	ethNodeURL := testConfig["MAINNET_URL"]
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
	gasGauge, err := ethgas.NewGasGauge(nil, monitor)
	assert.NoError(t, err)

	err = gasGauge.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer gasGauge.Stop()

	sub := gasGauge.Subscribe()
	defer sub.Unsubscribe()

	// Wait for requests to complete
	select {
	case <-vcr.Done():
		break
	case <-time.After(1 * time.Minute): // max amount of time to run the vcr recorder
		break
	}

	gasGauge.Stop()
	monitor.Stop()

	// assertions
	suggestedGasPrice := gasGauge.SuggestedGasPrice()
	assert.Equal(t, uint64(0xc0), suggestedGasPrice.Instant)
	assert.Equal(t, uint64(0xa2), suggestedGasPrice.Fast)
	assert.Equal(t, uint64(0x85), suggestedGasPrice.Standard)
	assert.Equal(t, uint64(0x71), suggestedGasPrice.Slow)
	assert.Equal(t, uint64(0xb54969), suggestedGasPrice.BlockNum.Uint64())
	assert.Equal(t, uint64(0x602e5b5e), suggestedGasPrice.BlockTime)
}
