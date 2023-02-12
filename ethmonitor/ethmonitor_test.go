package ethmonitor_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/util"
	"github.com/go-chi/httpvcr"
	"github.com/stretchr/testify/assert"
)

func TestMonitorBasic(t *testing.T) {
	testConfig, err := util.ReadTestConfig("../ethkit-test.json")
	if err != nil {
		t.Error(err)
	}

	ethNodeURL := testConfig["GOERLI_URL"]
	if ethNodeURL == "" {
		ethNodeURL = "http://localhost:8545"
	}

	ctx := context.Background()

	vcr := httpvcr.New("ethmonitor_goerli")
	vcr.Start(ctx)
	defer vcr.Stop()

	vcr.URLRewriter = func(url string) string {
		// rewrite the url to hide the API keys
		return "http://goerli/"
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

	go func(t *testing.T) {
		err := monitor.Run(context.Background())
		if err != nil {
			panic(err)
		}
	}(t)
	defer monitor.Stop()

	sub := monitor.Subscribe()
	defer sub.Unsubscribe()

	subs := []ethmonitor.Subscription{}
	go func() {
		for i := 0; i < 10; i++ {
			s := monitor.Subscribe()
			subs = append(subs, s)
		}

		time.Sleep(1 * time.Second)
		for _, s := range subs {
			s.Unsubscribe()
		}
	}()

	go func() {
		for {
			select {
			case blocks := <-sub.Blocks():
				_ = blocks

				for _, b := range blocks {
					fmt.Println("event:", b.Event, "block:", b.NumberU64(), b.Hash().Hex(), "parent:", b.ParentHash().Hex(), "# logs:", len(b.Logs))
				}

				finalBlock := monitor.LatestFinalBlock(3)
				if finalBlock != nil {
					fmt.Println("finalized block #", finalBlock.NumberU64())
				}

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

	for _, b := range blocks {
		_ = b
		// fmt.Println("=> block", b.Hash())
	}

	expectedBlockHashes := []string{
		"0x4b620f3e2c4f610dcd35ca775b78d43ee00eb22c3478a298be85c33a9754860e",
		"0x6fef9cf6058711113fd64002df5e94da53a3221ad6c62efd319a713d29834c50",
		"0x9f5ec01f95d58827e8a57c37537f619447a30b49a6f61564ce6b3d31665ec1a4",
		"0xc3bad9a9aebf1dedd4ad53c2ae284515708c8602cd5f29da98add6fc3bf90f34",
		"0xaced539b970190b3ed9ec05d83c0346c37a604b8dea3b91d9246201940a750ba",
		"0xf5b3c806c7ff8c4a2edc3d66548ec5bb07603c252a47421396a8446db9b57654",
		"0x2f9440d468c060060a432ae772d64f7caa85351a8c15d5587413e92110c5c867",
		"0xe0490515b4796de3fe6ca8eae4c8457443405fa6237d59c355935382781aae61",
		"0x3431e89e2c9a8327e2fb5a0f167daa198bc1f74b8c27c45f94f699b7bd5a80b1",
		"0x81945d5d3410e819cf7351589603199c970deb01cc2832601c5ea10dc2f74bf0",
		"0xe2148ee53784ce6b717cc2955c9b24c810ea7c437e04a05044e046df5cb3c67d",
		"0x1a2650fb073998f2b850c680efa774d7fcee26cd0ace7354603d2a62318284bb",
		"0xef10f72d4c9bc0913b4f7296b7bfad4bfdde7a68227fd91329663ce6c6a59fb5",
		"0xa5c09c1fcd88eafbfb96cafedbc10357a6b583aae071498ed1a62549c717a2b5",
		"0x8e14ad887c4eaf2112f4eefa569e4be30eb920a7f86cbbed2f5aede383f6f4da",
		"0xc83ea7430aedf18c124481a518b573ace7497d199aece7638b3beef2f4ba3052",
		"0xb1d491622e065e8f9e5d2c5d26a47e6f1dddb46e98ea1f650b2e53b989a54077",
		"0x332b10a068e34e6d632c0012bf4dd43c8672c6371aef729633996e5af5c3a662",
		"0x6669ce675f8e81bba87cb0e85196a895708b16dd630f21f05dade3ece9b27a8a",
		"0xc53170fe270cbfb228fd37c7a08b24732ab1b78d49167198d91139a24fa98c8f",
		"0x7fa000d70220f3b73d35e40d91143bc5e16f9731d1b692d077288d282bf62fa1",
		"0x7b93ffc766e16e09b9873c28a85a59c7fe6bc3c286dbc3234fc8881d715c14ac",
		"0x8a99f2b3390b68685ed39ff098926e8cabde4ac2647f70beb117d55c5c425127",
	}

	assert.True(t, len(expectedBlockHashes) <= len(blocks), "expected blocks returned part of retention")

	for i := range expectedBlockHashes {
		assert.Equal(t, expectedBlockHashes[i], blocks[i].Hash().Hex())
	}

	assert.NotNil(t, monitor.GetBlock(common.HexToHash("0xc53170fe270cbfb228fd37c7a08b24732ab1b78d49167198d91139a24fa98c8f")))
	assert.Equal(t, common.HexToHash("0x8a99f2b3390b68685ed39ff098926e8cabde4ac2647f70beb117d55c5c425127"), monitor.LatestBlock().Hash())

	// only subscriber left is the main one
	assert.True(t, monitor.NumSubscribers() == 1)
}

func GetIp(index uint) string {
	output, err := exec.Command("yarn", "--silent", "--cwd", "../tools/reorgme", "chain:ip", "0").CombinedOutput()

	if err != nil {
		os.Stderr.WriteString(err.Error())
	}

	return strings.Replace(string(output), "\n", "", 1)
}

func WaitBlock(ctx context.Context, provider *ethrpc.Provider) error {
	var lastBlock = uint64(0)

	fmt.Println("Waiting a block")

	for {
		block, err := provider.BlockNumber(ctx)
		if err != nil {
			return err
		}

		if lastBlock == 0 {
			lastBlock = block
		}

		if block != lastBlock {
			return nil
		}
	}
}

func Fork(index uint) string {
	fmt.Println("Forking...")
	output, err := exec.Command("yarn", "--silent", "--cwd", "../tools/reorgme", "chain:fork").CombinedOutput()

	if err != nil {
		os.Stderr.WriteString(err.Error())
	}

	fmt.Println("Forked!")

	return string(output)
}

func Join(index uint) string {
	fmt.Println("Joining...")
	output, err := exec.Command("yarn", "--silent", "--cwd", "../tools/reorgme", "chain:join").CombinedOutput()

	if err != nil {
		os.Stderr.WriteString(err.Error())
	}

	fmt.Println("Joined!")

	return string(output)
}

func TestMonitorWithReorgme(t *testing.T) {
	if strings.ToLower(os.Getenv("REORGME")) != "true" {
		t.Logf("REORGME is not enabled, skipping this test case.")
		return
	}

	ip := GetIp(0)

	provider, err := ethrpc.NewProvider("http://" + ip + ":8545/")
	assert.NoError(t, err)

	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.PollingInterval = 5 * time.Millisecond

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	go func(t *testing.T) {
		err := monitor.Run(context.Background())
		if err != nil {
			panic(err)
		}
	}(t)
	defer monitor.Stop()

	sub := monitor.Subscribe()
	defer sub.Unsubscribe()

	events := make([]*ethmonitor.Block, 0)

	go func() {
		for {
			select {
			case blocks := <-sub.Blocks():
				_ = blocks
				for _, b := range blocks {
					events = append(events, b)
					fmt.Println("event:", b.Event, "block:", b.NumberU64(), b.Hash().Hex(), "parent:", b.ParentHash().Hex(), "# logs:", len(b.Logs))
				}
			case <-sub.Done():
				return
			}
		}
	}()

	Fork(0)
	events = make([]*ethmonitor.Block, 0)

	WaitBlock(context.Background(), provider)
	WaitBlock(context.Background(), provider)

	time.Sleep(2 * time.Second)

	for _, b := range events {
		assert.Equal(t, b.Event, ethmonitor.Added)
	}

	revertedEvents := events
	events = make([]*ethmonitor.Block, 0)

	Join(0)

	// Wait for reorg
	WaitBlock(context.Background(), provider)
	WaitBlock(context.Background(), provider)

	time.Sleep(2 * time.Second)

	offset := 0
	for _, e := range events {
		if e.Block.Hash() == revertedEvents[len(revertedEvents)-1].Hash() {
			break
		}

		offset++
	}

	for i, b := range revertedEvents {
		ri := len(revertedEvents) - 1 - i + offset
		rb := events[ri]

		// Should revert last blocks
		assert.Equal(t, rb.Block.Number(), b.Block.Number())
		assert.Equal(t, rb.Block.Hash(), b.Block.Hash())
		assert.Equal(t, b.Event, ethmonitor.Removed)
	}

	monitor.Stop()
}
