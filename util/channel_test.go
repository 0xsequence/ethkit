package util_test

import (
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/util"
	"github.com/goware/logger"
	"github.com/stretchr/testify/assert"
)

func TestSlowProducer(t *testing.T) {
	testUnboundedBufferedChannel(t, 100*time.Millisecond, 0, 10)
}

func TestSlowConsumer(t *testing.T) {
	testUnboundedBufferedChannel(t, 0, 100*time.Microsecond, 100)
}

func testUnboundedBufferedChannel(t *testing.T, producerDelay time.Duration, consumerDelay time.Duration, messages int) {
	ch := make(chan ethmonitor.Blocks)
	sendCh := util.MakeUnboundedChan(ch, logger.NewLogger(logger.LogLevel_INFO), 100)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		expected := 0
		for blocks, ok := <-ch; ok; blocks, ok = <-ch {
			fmt.Printf("received message %v\n", blocks[0].Number())
			time.Sleep(consumerDelay)
			assert.Equal(t, expected, int(blocks[0].NumberU64()))
			expected++
		}

		assert.Equal(t, messages, expected)
		wg.Done()
	}()

	for i := 0; i < messages; i++ {
		fmt.Printf("sending message %v\n", i)
		header := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(int64(i))})
		sendCh <- ethmonitor.Blocks{&ethmonitor.Block{Block: header}}
		time.Sleep(producerDelay)
	}

	close(sendCh)
	wg.Wait()
}
