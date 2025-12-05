package ethutil

import (
	"bytes"

	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// LogsBloomCheckFunc allows callers to override how logs bloom validation is performed.
// Returning true means the logs match the header; false means they do not.
type LogsBloomCheckFunc func(logs []types.Log, header *types.Header) bool

// ValidateLogsWithBlockHeader validates that the logs comes from given block.
// If the list of logs is not complete or the logs are not from the block, it
// will return false.
func ValidateLogsWithBlockHeader(logs []types.Log, header *types.Header, optLogsBloomCheck ...LogsBloomCheckFunc) bool {
	// Allow callers to override the check logic (e.g. filtering certain logs).
	if len(optLogsBloomCheck) > 0 && optLogsBloomCheck[0] != nil {
		return optLogsBloomCheck[0](logs, header)
	}

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
