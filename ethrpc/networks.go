package ethrpc

type Network struct {
	Name                string
	ChainID             uint64
	NumBlocksToFinality int
	OptimismChain       bool
}

var Networks = map[uint64]Network{
	1: {
		Name:                "mainnet",
		ChainID:             1,
		NumBlocksToFinality: 20,
	},
	3: {
		Name:                "ropsten",
		ChainID:             3,
		NumBlocksToFinality: 20,
	},
	4: {
		Name:                "rinkeby",
		ChainID:             4,
		NumBlocksToFinality: 20,
	},
	5: {
		Name:                "goerli",
		ChainID:             5,
		NumBlocksToFinality: 20,
	},
	42: {
		Name:                "kovan",
		ChainID:             42,
		NumBlocksToFinality: 20,
	},
	11155111: {
		Name:                "sepolia",
		ChainID:             11155111,
		NumBlocksToFinality: 50,
	},
	137: {
		Name:                "polygon",
		ChainID:             137,
		NumBlocksToFinality: 100,
	},
	80001: {
		Name:                "polygon-mumbai",
		ChainID:             80001,
		NumBlocksToFinality: 100,
	},
	56: {
		Name:                "bsc",
		ChainID:             56,
		NumBlocksToFinality: 50,
	},
	97: {
		Name:                "bsc-testnet",
		ChainID:             97,
		NumBlocksToFinality: 50,
	},
	10: {
		Name:                "optimism",
		ChainID:             10,
		NumBlocksToFinality: 50,
		OptimismChain:       true,
	},
	69: {
		Name:                "optimism-testnet",
		ChainID:             69,
		NumBlocksToFinality: 50,
		OptimismChain:       true,
	},
	42161: {
		Name:                "arbitrum",
		ChainID:             42161,
		NumBlocksToFinality: 50,
	},
	421613: {
		Name:                "arbitrum-testnet",
		ChainID:             421613,
		NumBlocksToFinality: 50,
	},
	42170: {
		Name:                "arbitrum-nova",
		ChainID:             42170,
		NumBlocksToFinality: 50,
	},
	43114: {
		Name:                "avalanche",
		ChainID:             43114,
		NumBlocksToFinality: 50,
	},
	43113: {
		Name:                "avalanche-testnet",
		ChainID:             43113,
		NumBlocksToFinality: 50,
	},
	250: {
		Name:                "fantom",
		ChainID:             250,
		NumBlocksToFinality: 100,
	},
	4002: {
		Name:                "fantom-testnet",
		ChainID:             4002,
		NumBlocksToFinality: 100,
	},
	100: {
		Name:                "gnosis",
		ChainID:             100,
		NumBlocksToFinality: 100,
	},
	1313161554: {
		Name:                "aurora",
		ChainID:             1313161554,
		NumBlocksToFinality: 50,
	},
	1313161556: {
		Name:                "aurora-testnet",
		ChainID:             1313161556,
		NumBlocksToFinality: 50,
	},
	8453: {
		Name:                "base",
		ChainID:             8453,
		NumBlocksToFinality: 50,
		OptimismChain:       true,
	},
	84531: {
		Name:                "base-goerli",
		ChainID:             84531,
		NumBlocksToFinality: 50,
		OptimismChain:       true,
	},
	19011: {
		Name:                "homeverse",
		ChainID:             19011,
		NumBlocksToFinality: 50,
		OptimismChain:       true,
	},
	40875: {
		Name:                "homeverse-testnet",
		ChainID:             40875,
		NumBlocksToFinality: 50,
		OptimismChain:       true,
	},
}
