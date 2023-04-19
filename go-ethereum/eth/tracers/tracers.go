package tracers

import (
	"encoding/json"

	"github.com/0xsequence/ethkit/go-ethereum/eth/tracers/logger"
	"github.com/0xsequence/ethkit/go-ethereum/internal/ethapi"
)

type TraceCallConfig struct {
	TraceConfig
	StateOverrides *ethapi.StateOverride
	BlockOverrides *ethapi.BlockOverrides
}

type TraceConfig struct {
	*logger.Config
	Tracer  *string
	Timeout *string
	Reexec  *uint64
	// Config specific to given tracer. Note struct logger
	// config are historically embedded in main object.
	TracerConfig json.RawMessage
}
