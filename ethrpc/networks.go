package ethrpc

type Network struct {
	Name                string
	ChainID             uint64
	NumBlocksToFinality int
}

var Networks = map[uint64]Network{
	1: {
		Name:                "mainnet",
		ChainID:             1,
		NumBlocksToFinality: 20,
	},
	//....
}
