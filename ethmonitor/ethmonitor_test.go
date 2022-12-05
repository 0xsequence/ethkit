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

	go func() {
		for {
			select {
			case blocks := <-sub.Blocks():
				_ = blocks
				for _, b := range blocks {
					fmt.Println("event:", b.Event, "block:", b.NumberU64(), b.Hash().Hex(), "parent:", b.ParentHash().Hex(), "# logs:", len(b.Logs))
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
		"0xb2370c551d49233cbc9a820f9e0182065d3ee094e7ff2ac4478c260645aa75b8",
		"0x46390a14c0f54cd2f75dbdd01221ff6597074fd08ab74b16d62515942ea2248e",
		"0xe9be1a52123aa9edbee8e24d88984c7307e5cd1b8dce3586907ed0e19589c561",
		"0x038eff01ab04935aeb44f9114cd97e9f65db2a77509f9bc056bb070a3d8d7fd3",
		"0x7799d84662563d480ab98eb0dc0dee137848615e7edef4e9baff69e4e5abec63",
		"0xa3b5f36392ed9e5ba305bc733cee5f34546b20bd83caa1d6302f380ec18c348f",
		"0x514596ba6fef21b084fe99a62320ec08144447a7573dbdec3867014f0b9e98bd",
		"0x2e5084449e303eebc9304c640f0a2c38d342cab18583445329e8e416a442f88b",
		"0x5be2f519fc9598576c30bebd76d221019bd1ff94bc647327f5c63f8bddb2ff02",
		"0x2db8db3af56a521fcdd6490344f0cce4a73a4c9cd885ac3b137ff2b39c0596b8",
		"0xaaaf713c03047f1e8532b38a05e7fb4c396368842c364b10075a0566bdac2993",
		"0x30a08f34e196806ebc4550ad2e1f5be560dfc4db55bcf6e062e7137d0985cee8",
		"0xfcf9e33633b2eaa55c23f047c9aa0e6b2d5ee9841c4517feb37563a3d0703505",
		"0xa8cea127e357f4e131e30ca7540ca52a7bf2913f861d706c77631efc3cf8d380",
		"0x1467d98a98c7bf50dfb6d4da9a7a7870ca98377365456c725056653204e2b9f6",
		"0x429d6a79a938152512a74c7e3fa4097cc3d3fc5f96d54c08708286a76321cffd",
		"0xa3f5afe04b2c570dc2d1612d2de688d7b01b58d01d6d3bcae70a2016c9ef55bb",
		"0xb5c5aeaba3b82b582d9ec6c2f4543f70ef0b0c6083b74fcc3b864434752c7d42",
		"0xc9b62fbcefd934b265f20326ab88ebef6b0e9ce307768de15c6220a0b44fd17d",
		"0x5d0497ff63b5cf4f4208d4a2b8744ce0524ab7086db5d115e8e2eec77b35e424",
		"0x7dfe33b93aceb8aa3796ec79579d4d6b4a4cd986b7c87c19e24b0c86e971dc2b",
		"0x1554bde841a8029c173efe1a2bfcff645cf61821a9e50e3bcedda368c5666c2f",
		"0xa26d7d66e2321d9481287c1cebf484defe6b2341ac6cc7eca0ab3a40bc75ba1a",
	}

	assert.True(t, len(expectedBlockHashes) <= len(blocks), "expected blocks returned part of retention")

	for i := range expectedBlockHashes {
		assert.Equal(t, expectedBlockHashes[i], blocks[i].Hash().Hex())
	}

	assert.NotNil(t, monitor.GetBlock(common.HexToHash("0x5d0497ff63b5cf4f4208d4a2b8744ce0524ab7086db5d115e8e2eec77b35e424")))
	assert.Equal(t, common.HexToHash("0xa26d7d66e2321d9481287c1cebf484defe6b2341ac6cc7eca0ab3a40bc75ba1a"), monitor.LatestBlock().Hash())
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
