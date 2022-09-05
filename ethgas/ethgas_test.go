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
	// monitorOptions.StrictSubscribers = false
	if vcr.Mode() == httpvcr.ModeReplay {
		// change options to run replay tests faster
		monitorOptions.PollingInterval = 100 * time.Millisecond
	}

	// Setup provider and monitor
	provider, err := ethrpc.NewProvider(ethNodeURL)
	assert.NoError(t, err)

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	go func() {
		err := monitor.Run(ctx)
		if err != nil {
			panic(err)
		}
	}()
	defer monitor.Stop()

	// Setup gas tracker
	gasGauge, err := ethgas.NewGasGauge(util.NewLogger(util.LogLevel_DEBUG), monitor, 1, false)
	assert.NoError(t, err)

	go func() {
		err := gasGauge.Run(ctx)
		if err != nil {
			panic(err)
		}
	}()
	defer gasGauge.Stop()

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
	assert.Equal(t, "13068011406", suggestedGasPrice.InstantWei.String())
	assert.Equal(t, "13068011406", suggestedGasPrice.FastWei.String())
	assert.Equal(t, "13068011406", suggestedGasPrice.StandardWei.String())
	assert.Equal(t, "11107809695", suggestedGasPrice.SlowWei.String())
	assert.Equal(t, uint64(13), suggestedGasPrice.Instant)
	assert.Equal(t, uint64(13), suggestedGasPrice.Fast)
	assert.Equal(t, uint64(13), suggestedGasPrice.Standard)
	assert.Equal(t, uint64(11), suggestedGasPrice.Slow)
	assert.Equal(t, uint64(0xec2dac), suggestedGasPrice.BlockNum.Uint64())
	assert.Equal(t, uint64(0x6315fd96), suggestedGasPrice.BlockTime)
}
