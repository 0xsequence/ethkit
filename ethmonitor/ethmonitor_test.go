package ethmonitor_test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/goware/httpvcr"
	"github.com/arcadeum/ethkit/ethmonitor"
	"github.com/arcadeum/ethkit/ethrpc"
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
		monitorOptions.PollingInterval = 5 * time.Millisecond
	}

	provider, err := ethrpc.NewProvider(ethNodeURL)
	assert.NoError(t, err)

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	err = monitor.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer monitor.Stop()

	sub := monitor.Subscribe()
	defer sub.Unsubscribe()

	go func() {
		for {
			select {
			case blocks := <-sub.Blocks():
				_ = blocks
				// for _, b := range blocks {
				// 	fmt.Println("type:", b.Type, "block:", b.NumberU64(), b.Hash().Hex(), "parent:", b.ParentHash().Hex(), "# logs:", len(b.Logs))
				// }
			case <-sub.Done():
				return
			}
		}
	}()

	// Wait for requests to complete
	select {
	case <-vcr.Done():
		break
	case <-time.After(5 * time.Minute): // max amount of time to run the vcr recorder
		break
	}
	monitor.Stop()

	// Perform assertions
	blocks := monitor.Chain().Blocks()

	expectedBlockHashes := []string{
		"0x7bdfbd53820e7d6763c021c22fa30589370943e0862538ed4144d9c1b6861d9b",
		"0xacd4cb8d45bad229f34103273c8b057754bdee750ef607f7b1410ee845ffe588",
		"0x03075424551394c65d93f14326aeaf4c9c1aec5c4c15c054c4f05a8fa5b8269b",
		"0xcd2449dd08d9f30842055daad4c5e95b5d09b4926388be88d9e7e77817053719",
		"0x2b7dd1fa5cd475a34c4c96c1cc395a3bd0ea34580938fcbb1eaa4845d519fcb2",
		"0x1854423ebaaabda4987db0d687db153383c474733f60c4bd55f0211f1c471119",
		"0x3fcb30ebfa264f682552e16b7f949be331555047f5bf018464c67e44126f7cd1",
		"0x2f35920a68b336aea818996439684c9afbcfc7c5931b27a836f2aee9c14c7a5b",
		"0xd0eda8ef568328b8a2e3917e827939209a287e017c350062086bf092c53f3a22",
		"0x27ba0fae46733d033b31ca49611180ea009bdcddc4d9566c7afbddebac56c7b5",
		"0x012bcdf33468b13eb01419a654729b071da483558ca244f1d728d00901642267",
		"0xe8ec36c0edf933a4375ec47561c2db1c022d00e936f013299562beed8f9f6386",
		"0x9d7343dc5e9778ca11037cbf5582daee32eed93d9d008f6764c8a486bd557724",
		"0xa106bcf2afab3688faca5868bdb0a1672b5fd62f66d89a4528e84a525938d601",
		"0x2e9a18c81e4ce2d01220daad226363c570039f111dceb7eca02a129d10ccdd19",
		"0xc33314ea51fb9dfa90936da3ab6ed588e7fc7b61c7a4b122114a3f178e02d1fd",
		"0xbf61f38663fe2d3cc6d8777e1b0b2236195d96aecafc3ee9a187076fde19d7a7",
		"0x328838071b9059e24abdb615f54d15e75e60d250cd0857ffc5bf9fd4d334505b",
		"0xe380bf5902a75e4e8f8dd91b3274d24a84d742e31a01e2d322f974a1a4590649",
		"0xd163227d88fc3ec272278eefe9898b9606d678b6cfa63d0c6db6c251a0514241",
	}

	assert.Len(t, blocks, len(expectedBlockHashes))

	for i := range blocks {
		assert.Equal(t, expectedBlockHashes[i], blocks[i].Hash().Hex())
	}

	assert.NotNil(t, monitor.GetBlock(common.HexToHash("0xc33314ea51fb9dfa90936da3ab6ed588e7fc7b61c7a4b122114a3f178e02d1fd")))
	assert.Equal(t, common.HexToHash("0xd163227d88fc3ec272278eefe9898b9606d678b6cfa63d0c6db6c251a0514241"), monitor.LatestBlock().Hash())
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
