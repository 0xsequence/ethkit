package ethutil

import (
	"bytes"

	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// ValidateLogsWithBlockHeader validates that the logs comes from given block.
// If the list of logs is not complete or the logs are not from the block, it
// will return false.
func ValidateLogsWithBlockHeader(logs []types.Log, header *types.Header) bool {
	return bytes.Compare(logsToBloom(logs).Bytes(), header.Bloom.Bytes()) == 0
}

func logsToBloom(logs []types.Log) types.Bloom {
	var logBloom types.Bloom
	for _, log := range logs {
		logBloom.Add(log.Address.Bytes())
		for _, b := range log.Topics {
			logBloom.Add(b[:])
		}
	}
	return logBloom
}
