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
		monitorOptions.PollingInterval = 5 * time.Millisecond
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
		"0xaf8d2a88dae2249f55b95e851ba7d5bd7fffad53bf98594a917207083fa5e43e",
		"0xa7191de960951e9f9551c778d92a2ca2ef973639f882710f10f18435b2024cba",
		"0x23e6b0e9b5f38efb1329ae64879a815c599b77f5bfc6d348d6949fe6724c58b9",
		"0xe7433c34e5e01d962c9f9f2932014209e1085aa904b1b90ca7b900ae514c990b",
		"0xeeb9f5328f462718d1a1dfc883dd958fc2461ca594a210e62db1467022464871",
		"0xf76493f0c6405466f622413f915226497f1a4f0f0b80b47369b0e9044858ec8d",
		"0x594615058a1921c197b8cd5d1132cd02f270a8e60ec86c63ece52275b7d0c10f",
		"0x9e7044a454e39915e20d9d8340ee20f53e6675184926701dea97e8624fa97b9c",
		"0x3d0cb9933a5340daf667b9f080932ee9595cfadb0c39f2fe42bfdab238e48446",
		"0x65a2918186d19a0b8ef4733f8608f94b375465a4ede009dcb043734d6ddb2053",
		"0xe5a1d2c11681e2497ef5f29e349a6aac349ecda5b98b91fe165b3dec86380511",
		"0x00b458cb0be74863649bdf6686218573a5a5d3fb59ae3d9b70127d7ad3158aa5",
		"0xd37a817d36efbf748ea3df0252b73e7c93c721b9a06c6c48f409481553af011c",
		"0x26398058afcc06876d6b1e0c04288006b619b046be22364b976e91b5a0dcf8dd",
		"0x03f2e67553362c7af23311a21bed36dc5a6bcfb0634bae4f3265dd9dd30a89ee",
		"0x5aea42886072a5ccc0e2b1ff2dccc8640c5804e4fceb539e09c9fb3a8bde1eee",
		"0x85ecad34f6787c6009fe54c9d1fe71840638fe983bf4fda8f438491fdd368393",
		"0x31dabb41965a70f76685ef30a60ed7fc4c9cf9fdf5e1cf9f3c6011de4a9676a6",
		"0x4d3d9a249f75ec319a4f5b1e37334056b19fe6a9c6978482b346877072b76d8d",
		"0x3d7630741497e658970ba390b129310802d1d45773d745e855c9248f1b58afcd",
	}

	assert.Len(t, blocks, len(expectedBlockHashes))

	for i := range blocks {
		assert.Equal(t, expectedBlockHashes[i], blocks[i].Hash().Hex())
	}

	assert.NotNil(t, monitor.GetBlock(common.HexToHash("0x26398058afcc06876d6b1e0c04288006b619b046be22364b976e91b5a0dcf8dd")))
	assert.Equal(t, common.HexToHash("0x3d7630741497e658970ba390b129310802d1d45773d745e855c9248f1b58afcd"), monitor.LatestBlock().Hash())
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
