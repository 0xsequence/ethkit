package ethmonitor_test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/goware/httpvcr"
	"github.com/horizon-games/ethkit/ethmonitor"
	"github.com/horizon-games/ethkit/ethrpc"
	"github.com/stretchr/testify/assert"
)

func TestMonitor(t *testing.T) {
	testConfig := readTestFile(t)
	ethNodeURL := testConfig["INFURA_RINKEBY_URL"]
	if ethNodeURL == "" {
		ethNodeURL = "http://localhost:8545"
	}

	ctx := context.Background()

	vcr := httpvcr.New("ethmonitor_rinkeby")
	vcr.Start(ctx)
	defer vcr.Stop()

	vcr.URLRewriter = func(url string) string {
		// rewrite the url to hide the API keys
		return "http://rinkeby/"
	}

	monitorOptions := ethmonitor.DefaultOptions
	if vcr.Mode() == httpvcr.ModeReplay {
		// change options to run replay tests faster
		monitorOptions.BlockPollTime = 5 * time.Millisecond
	}

	provider, err := ethrpc.NewJSONRPC(ethNodeURL)
	assert.NoError(t, err)

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	err = monitor.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer monitor.Stop()

	// Wait for requests to complete
	select {
	case <-vcr.Done():
		break
	case <-time.After(20 * time.Second): // max amount of time to run the vcr recorder
		break
	}
	monitor.Stop()

	// Perform assertions
	blocks := monitor.Chain().Blocks()
	assert.Len(t, blocks, 2)

	// TODO: re-run the recorder for 9 minutes to get a shit ton more data

	assert.Equal(t, "0x763a2442d134c4dce595c8cdcc696ba6489f558a746dbe8689ddbe0b0c7fa32b", blocks[0].Hash().Hex())
	assert.Equal(t, "0x553be8f8248c01c403df246ccb9be7d06db2b5fc08b2c33003e53403f3820f11", blocks[1].Hash().Hex())
}

func readTestFile(t *testing.T) map[string]string {
	config := map[string]string{}
	testFile := "../ethkit-test.json"

	_, err := os.Stat(testFile)
	if err != nil {
		return config
	}

	data, err := ioutil.ReadFile("../ethkit-test.json")
	if err != nil {
		t.Fatalf("%s file could not be read", testFile)
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		t.Fatalf("%s file json parsing error", testFile)
	}

	return config
}
