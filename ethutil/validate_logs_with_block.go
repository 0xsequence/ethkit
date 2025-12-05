package ethutil

import (
	"bytes"

	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// LogsBloomCheckFunc is the shape of a logs bloom validation function.
// Returning true means the logs match the header; false means they do not.
type LogsBloomCheckFunc func(logs []types.Log, header *types.Header) bool

// LogsFilterFunc transforms or filters logs before validation.
// The block is provided for cases where filtering depends on transaction data.
type LogsFilterFunc func(logs []types.Log, header *types.Header, block *types.Block) []types.Log

// ValidateLogsWithBlockHeader validates that the logs come from the given block
// by comparing the calculated bloom against the header bloom.
func ValidateLogsWithBlockHeader(logs []types.Log, header *types.Header) bool {
	return bytes.Equal(ConvertLogsToBloom(logs).Bytes(), header.Bloom.Bytes())
}

func ConvertLogsToBloom(logs []types.Log) types.Bloom {
	var logBloom types.Bloom
	for _, log := range logs {
		logBloom.Add(log.Address.Bytes())
		for _, b := range log.Topics {
			logBloom.Add(b[:])
		}
	}
	return logBloom
}
