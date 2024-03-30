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
		"0x7f06fd3877f4d664c4525d33a0ada20287918dd0280b372ca859481bd09c81c4",
		"0x0c4430bce8b4498657b88646827318c5d35cc52d3992d1fb97c28bcd69f0ff20",
		"0x9e385c0a6a749f2b51b7ccf0412d02cb4e82e2600c56477b750fd580e5c98273",
		"0x83fdf84b3cbfb4b0093025f9cc2db974e865d121b67dafafb70c2343837ccee2",
		"0xdc482579c13aa25d0ae9ade680751ef8f80cf771b29ea9d3bb47af2bb474e251",
		"0x2a45220ddbb622dad9107e84f3694b7fb03c52ced9e61e7293240595ffd5e620",
		"0x4b45986f0409622d1fe90f476357135031aa5eee7d73586393c42524d20c1c12",
		"0x377741ca51322567ab4d8f72226fd8d282bfc0c31cd506370430d71686c63641",
		"0x0a096c3f639e40cdbf06dab7fca394310ddd689456aa0131edd94a495f706e68",
		"0x56f1c94e1f3b466e4326f248e43d99d1bc8f55bc3147315a361dd4986fa5bd93",
		"0x420d67c7494ab8ab878d4cbd66a451d60491794b54f3bb16bbbb76ac48f15072",
		"0x8e00e1a3938f01b255e1cef3d8083730058745f0ae98f5cebc654d7ed6db3dbb",
		"0xad24f03ed8bdfb9637232384e3257e076a47f3ac583492e8463a8d1a5fcdd31f",
		"0x1147f217faa9a366f623d02453d099e60ddb30df36ec7ef9e80cc21adb43a730",
		"0x253b4383d2e9afa7dbba30da47e44a691bd804114a10b37b803881fdc380da2a",
		"0xb4cff922dd7cd5257acfdc433d672743dd1673030869da6f2278d4358803cf3c",
		"0x5bc9ab2442688acb26b8e941660c62915418627452807367b9919011c4808b24",
		"0x055ed7914285e38e30c69ae95bea80b49490a68d4057bd24b44b5444fcf34e70",
		"0x9120c53b4d31cc0188e7e49b201df5ce091655491e37820f613cf7d44bb93bd7",
		"0xdb1c79799e99a62c9dba44c7fb30e75f48da5f0ce67fdc4a2ecc08bf80bf52fb",
	}

	assert.True(t, len(expectedBlockHashes) <= len(blocks), "expected blocks returned part of retention")

	for i := range expectedBlockHashes {
		assert.Equal(t, expectedBlockHashes[i], blocks[i].Hash().Hex())
	}

	assert.NotNil(t, monitor.GetBlock(common.HexToHash("0x2a45220ddbb622dad9107e84f3694b7fb03c52ced9e61e7293240595ffd5e620")))
	assert.Equal(t, common.HexToHash("0xdb1c79799e99a62c9dba44c7fb30e75f48da5f0ce67fdc4a2ecc08bf80bf52fb"), monitor.LatestBlock().Hash())

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
