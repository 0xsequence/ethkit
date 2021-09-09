package ethmonitor_test // TODO: change to just ethmonitor test

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

func TestMonitor(t *testing.T) {
	testConfig, err := util.ReadTestConfig("../ethkit-test.json")
	if err != nil {
		t.Error(err)
	}

	ethNodeURL := testConfig["RINKEBY_URL"]
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

	expectedBlockHashes := []string{
		"0x49e3511c0c83502d9424eabe3dbc57bfe9f8a7281ab6b39a6d25c8b740a976f5",
		"0x3cc63b89dd1cd93579c2219f3c45848a5a5d167287d8d6b1f3336d95cfa07780",
		"0xdeae0e2dbd9cae35f09f96be00625070f3f76db73a9f47a4a0f32b72ffdae04f",
		"0xa1871b38e0e43ef322dcc76ee6e2d4d2230dff13eecf41f5c48e39918f331e78",
		"0xb5f148fb66d308f57f7f8a076f860356a40de86a29d7c02b6c66063671eddd35",
		"0xaff39f96c3cedafaadadc109a95c295d68a14083f440eaae5c582ef4152bb676",
		"0x0cfb489f804bb7b2c685f2c20197d052cbf30df041e2fb888ee524d3dcfb88ed",
		"0xaaeeef4d19833a4937a0948bfd6cc4c5b0a9bc6b10450e193de05cb9b2a2f7e0",
		"0xe087c47960f9a9ef38ef74fd3b3e925d153b58a7e861703bf6f9057a64b0b541",
		"0x1e9d061020a7a58daec91a32112d03b804e769d1cf797244d79d146f642bb6c7",
		"0x4ffb328e1fa23ec5de46530e4ae3bdac887e1275378bcc241b8d6e9fe410ac0f",
		"0x8ed1a0f57def48f562db577309732757a37de74ecfa53a0dcd3601a362c08f78",
		"0xa7da2f11da6bf7f4609b3dce1d544fda25b1f94cd3a8d537c3e077e19b2cdbdf",
		"0x0795e07b4abe520670df0b201e16e1cf2c918e998bfec93cd60aff543bb570eb",
		"0x29d3bf19ed823c014677293eb6fbe3868068acedf7a5b8020c64b4f29e5ba89d",
		"0x1201cb30d953a53aea8fe44291b7dce13a08931f05821ffcb2ef0ab97a9f188e",
		"0xc0d716d263393dc8f95557c6308e691a1c7560009a3a5e896758f66fe1fc73dd",
		"0x3cc6d0c6c49f4f2d0abc7e612a80b26e9fb4c91b91c5d27212979d786acab606",
		"0x06fbcbbda78ec602aa7b910f89ecc0f37fc6c7e9bb5863b3cc742a48df8ba6c0",
		"0xb053e4fff62415cba7096a00f4ef7622f5824e5725cc72e8e5c7d16d36f16278",
		"0x402dd2a2c103eb7d10194e8ca830b9bd7c5199ddd3f89bae814a435093035089",
	}

	assert.True(t, len(expectedBlockHashes) <= len(blocks), "expected blocks returned part of retention")

	for i := range expectedBlockHashes {
		assert.Equal(t, expectedBlockHashes[i], blocks[i].Hash().Hex())
	}

	assert.NotNil(t, monitor.GetBlock(common.HexToHash("0x1e9d061020a7a58daec91a32112d03b804e769d1cf797244d79d146f642bb6c7")))
	assert.Equal(t, common.HexToHash("0x402dd2a2c103eb7d10194e8ca830b9bd7c5199ddd3f89bae814a435093035089"), monitor.LatestBlock().Hash())
}

func GetIp(index uint) string {
	output, err := exec.Command("yarn", "--silent", "--cwd", "../tools/test-chain", "chain:ip", "0").CombinedOutput()

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
	output, err := exec.Command("yarn", "--silent", "--cwd", "../tools/test-chain", "chain:fork").CombinedOutput()

	if err != nil {
		os.Stderr.WriteString(err.Error())
	}

	fmt.Println("Forked!")

	return string(output)
}

func Join(index uint) string {
	fmt.Println("Joining...")
	output, err := exec.Command("yarn", "--silent", "--cwd", "../tools/test-chain", "chain:join").CombinedOutput()

	if err != nil {
		os.Stderr.WriteString(err.Error())
	}

	fmt.Println("Joined!")

	return string(output)
}

func TestMonitorWithReorgme(t *testing.T) {
	if strings.ToLower(os.Getenv("SKIP_REORGME")) == "true" {
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
