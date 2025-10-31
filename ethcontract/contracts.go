package ethcontract

import "github.com/0xsequence/ethkit/ethartifact"

var (

	//go:embed contracts/ERC20.json
	artifact_erc20 string

	// Contracts registry to have some contracts on hand during testing
	contractRegistry = ethartifact.NewContractRegistry()
)

func init() {
	contractRegistry.MustAdd(ethartifact.MustParseArtifactJSON(artifact_erc20))
	// TODO: erc721, erc1155
}

func GetContractArtifact(name string) (ethartifact.Artifact, bool) {
	return contractRegistry.Get(name)
}
