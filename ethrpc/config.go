package ethrpc

import "time"

var DefaultJSONRPCConfig = &Config{
	BlockTime: 15 * time.Second,
}

type Config struct {
	Nodes []NodeConfig

	ChaindID  int // expected ChainID so we confirm on connect..
	BlockTime time.Duration
	TestMode  bool
}

type NodeConfig struct {
	URL                 string
	MaxRequestPerSecond float64
}

func (c *Config) AddNode(nodeConfig NodeConfig) {
	c.Nodes = append(c.Nodes, nodeConfig)
}
