package ethmonitor_test

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/util"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-chi/httpvcr"
	"github.com/stretchr/testify/assert"
)

func TestMonitorBasic(t *testing.T) {
	testConfig, err := util.ReadTestConfig("../ethkit-test.json")
	if err != nil {
		t.Error(err)
	}

	ethNodeURL := testConfig["SEPOLIA_URL"]
	if ethNodeURL == "" {
		ethNodeURL = "http://localhost:8545"
	}

	ctx := context.Background()

	vcr := httpvcr.New("ethmonitor_sepolia", httpvcr.Options{
		HTTPDefaultOverride: false,
		GZipCassette:        false,
	})
	vcr.Start(ctx)
	defer vcr.Stop()

	vcr.URLRewriter = func(url string) string {
		// rewrite the url to hide the API keys
		return "http://sepolia/"
	}

	monitorOptions := ethmonitor.DefaultOptions
	if vcr.Mode() == httpvcr.ModeReplay {
		// change options to run replay tests faster
		monitorOptions.PollingInterval = 5 * time.Millisecond
	}

	vcrClient := &http.Client{
		Transport: vcr,
	}

	provider, err := ethrpc.NewProvider(ethNodeURL, ethrpc.WithHTTPClient(vcrClient))
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

	sub := monitor.Subscribe("TestMonitorBasic")
	defer sub.Unsubscribe()

	subs := []ethmonitor.Subscription{}
	go func() {
		for i := 0; i < 10; i++ {
			s := monitor.Subscribe(fmt.Sprintf("TestMonitorBasic/sub/%d", i))
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
	case <-time.After(1 * time.Minute): // max amount of time to run the vcr recorder
		break
	}
	monitor.Stop()

	// Perform assertions
	blocks := monitor.Chain().Blocks()

	for _, b := range blocks {
		fmt.Println("=> block", b.Hash())
	}

	expectedBlockHashes := []string{
		"0x9c4fd4704019ced206ddd8701f0a81f4acdfaeca451ee923c88583c5a23a154b",
		"0xb56c0bb366db15d8cf15f3b9c75100f97d4aaa5e52f539ab51350a7cc6dbcdee",
		"0xa47fa3e299f50ca4bc58228474e5b0f58be69a9c97ded99e3d745376981dfdb2",
		"0xd4c342feee8bff67d228b53c341fa37de29dae599c6f179f156d56834b52953d",
		"0x59a033f3381537e0f0b23217ce294115f99aa4c6f79ed2cd02c3edfff2177b50",
		"0x772ac7379b5fbec7292737ce4fe6fb7040bec8bace271f620ebd58f8df259839",
	}

	assert.True(t, len(expectedBlockHashes) <= len(blocks), "expected blocks returned part of retention")

	for i := range expectedBlockHashes {
		assert.Equal(t, expectedBlockHashes[i], blocks[i].Hash().Hex())
	}

	assert.NotNil(t, monitor.GetBlock(common.HexToHash("0xa47fa3e299f50ca4bc58228474e5b0f58be69a9c97ded99e3d745376981dfdb2")))
	assert.Equal(t, common.HexToHash("0x772ac7379b5fbec7292737ce4fe6fb7040bec8bace271f620ebd58f8df259839"), monitor.LatestBlock().Hash())

	// only subscriber left is the main one
	time.Sleep(1500 * time.Millisecond)
	assert.Equal(t, 1, monitor.NumSubscribers())
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

	sub := monitor.Subscribe("TestMonitorWithReorgme")
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

func TestMonitorFeeHistory(t *testing.T) {
	const N = 1

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet")
	assert.NoError(t, err)

	monitorOptions := ethmonitor.DefaultOptions
	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	runErr := make(chan error, 1)
	go func() {
		runErr <- monitor.Run(ctx)
	}()
	defer monitor.Stop()

	sub := monitor.Subscribe("TestMonitorFeeHistory")
	defer sub.Unsubscribe()

	addedBlocks := 0
	for addedBlocks < N {
		select {
		case blocks := <-sub.Blocks():
			for _, b := range blocks {
				if b.Event == ethmonitor.Added {
					addedBlocks++
				}
			}
		case err := <-runErr:
			if err != nil {
				t.Fatalf("monitor stopped early: %v", err)
			}
			t.Fatalf("monitor stopped early without error")
		case <-ctx.Done():
			t.Fatalf("timeout waiting for %v blocks: %v", N, ctx.Err())
		}
	}

	blocks := monitor.Chain().Blocks()
	if len(blocks) < N {
		t.Fatalf("expected at least %v blocks, got %v", N, len(blocks))
	}

	lastBlock := new(big.Int).Set(blocks[len(blocks)-1].Number())
	rewardPercentiles := []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100}

	localFeeHistory, err := monitor.FeeHistory(ctx, N, lastBlock, rewardPercentiles)
	assert.NoError(t, err)
	spew.Dump("monitor.FeeHistory", localFeeHistory)

	rpcFeeHistory, err := provider.FeeHistory(ctx, N, lastBlock, rewardPercentiles)
	assert.NoError(t, err)
	spew.Dump("eth_feeHistory", rpcFeeHistory)
}
