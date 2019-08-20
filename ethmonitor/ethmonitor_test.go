package ethmonitor_test

import (
	"context"
	"encoding/json"
	"fmt"
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

	vcr := httpvcr.New("ethmonitor_rinkeby2")
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

	ch := make(chan []ethmonitor.Event)
	unsub := monitor.Subscribe(ch)
	defer unsub()

	go func() {
		for {
			select {
			case events := <-ch:
				for _, ev := range events {
					fmt.Println("event:", ev.Type, "block:", ev.Block.NumberU64(), ev.Block.Hash().Hex(), "parent:", ev.Block.ParentHash().Hex(), "# logs:", len(ev.Block.Logs))
				}
				fmt.Println("")
				fmt.Println("")
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
		"0x5b3f52c930304d0e313942e3f838ecce2640d0d1ed486f54f1a00f4b98675511",
		"0x0c1f6cb6bc5390a35fcbc36b0b8a823888e0e434a4db275e35c4c7894c42af09",
		"0xe9cc518a4d0f73b9213d17652caff297abcc2d686f374640364195c7698ae759",
		"0x2ce324cb09a4fb40503c5efe375597cf7226a177ecb8b2d0b5f4d2c8403d26f5",
		"0xd9d04990aa69a1fa5a9dc6b9dc548cfbd2d299b6826401420b2b330dc3a96099",
		"0x746e3aa1f8b4de4f6801a933e54316d4895086c906c4303d4200750f993475e5",
		"0x99489c79a5bd35979c711da2954b5acea0e17ae79395b7ba7a65f5cd529e5f0c",
		"0x2e78daad534b8addd6095441f323d57ba731cd6e85aa73569dab8cbbb72336b9",
		"0x9f19696f14f5675bbf8c5fdecf039cb1bb460bc43a96a6fb9307d7120cd18b72",
		"0xfba53dbd2d21bf53fda22aca342c490c59cf274536fcb9241f9ed6da0e5a9077",
		"0x999b1296d209edddbc0eaac9a0f5cbe644f8b525761928b77ec3a1a85ed4951f",
		"0xa1c9357bcd62bda640b8d755759e32c23f9d2469431432f479bf2b33e7e4b758",
		"0x9664973dfc85423759f068326b86ecfff8646d3a2f15a0e17ec94b2079321e86",
		"0x6c88984d56928bc981bf989dc80b94e1d65abb668ee210fa80187697479ed014",
		"0xa2ffcc25bb92175964db323779ed410cd8157cc01634c032fdbb21d7465f03ff",
		"0x8cd4dc7c5ad10daf571ba3747ce8c00492a40f9171b8abd1d951e3d92800c4e1",
		"0xcf617f558cfb118a0ec85623c4a87da177d6a3dbb2d75db921d1ec2c6ae8f59c",
		"0x2a96d71483b230b41b82d16dfd6186f77d6888d43101e232f1e979982d238f12",
		"0x3ac0451367c6899be0a4304873338e62c8a7f8b127c25b55980a00cafd867fd2",
		"0xc1c6a27168e371a461f5eb05485fba5d0b6e8f99daa275344665b173340b9584",
	}

	assert.Len(t, blocks, len(expectedBlockHashes))

	for i := range blocks {
		assert.Equal(t, expectedBlockHashes[i], blocks[i].Hash().Hex())
	}
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
