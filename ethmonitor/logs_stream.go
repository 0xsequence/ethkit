package ethmonitor

import (
	"context"
	"log/slog"
	"time"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type LogsStream struct {
	provider ethrpc.Interface
}

func NewLogsStream(provider ethrpc.Interface) *LogsStream {
	return &LogsStream{
		provider: provider,
	}
}

// func (s *LogsStream) StreamLogs(ctx context.Context) (<-chan [][]types.Log, <-chan error) {
// 	logsCh := make(chan [][]types.Log)
// 	errCh := make(chan error, 1)
//
// 	go func() {
// 		defer close(logsCh)
// 		defer close(errCh)
//
// 	}()
//
// 	return logsCh, errCh
// }

func (s *LogsStream) Do(ctx context.Context) {
	ch := make(chan types.Log)

	sub, err := s.provider.SubscribeFilterLogs(ctx, ethereum.FilterQuery{}, ch)
	if err != nil {
		panic(err)
	}
	defer sub.Unsubscribe()

	go func() {
		for {
			select {
			case err := <-sub.Err():
				panic(err)
			case log := <-ch:
				slog.Info("log received", "block", log.BlockNumber, "txIndex", log.TxIndex, "logIndex", log.Index)
			}

			// TODO: can a block tell us how many txns there are...?
			// yes..
		}
	}()

	time.Sleep(50 * time.Second)
}
