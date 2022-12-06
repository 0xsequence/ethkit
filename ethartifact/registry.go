package ethartifact

import (
	"fmt"
	"sort"

	"github.com/0xsequence/ethkit/ethcontract"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
)

func NewContractRegistry() *ContractRegistry {
	return &ContractRegistry{
		contracts: map[string]Artifact{},
		names:     []string{},
	}
}

type ContractRegistry struct {
	contracts map[string]Artifact
	names     []string // index of contract names in the map
}

func (c *ContractRegistry) Add(artifact Artifact) error {
	if c.contracts == nil {
		c.contracts = map[string]Artifact{}
	}
	if artifact.ContractName == "" {
		return fmt.Errorf("unable to register contract with empty name")
	}
	c.contracts[artifact.ContractName] = artifact
	c.names = append(c.names, artifact.ContractName)
	sort.Sort(sort.StringSlice(c.names))
	return nil
}

func (c *ContractRegistry) Register(contractName string, contractABI abi.ABI, contractBin []byte) (Artifact, error) {
	r := Artifact{ContractName: contractName, ABI: contractABI, Bin: contractBin}
	err := c.Add(r)
	if err != nil {
		return Artifact{}, err
	}
	return r, nil
}

func (s *ContractRegistry) RegisterJSON(contractName string, contractABIJSON string, contractBin []byte) (Artifact, error) {
	parsedABI, err := ethcontract.ParseABI(contractABIJSON)
	if err != nil {
		return Artifact{}, err
	}
	return s.Register(contractName, parsedABI, contractBin)
}

func (c *ContractRegistry) MustAdd(contractABI Artifact) {
	err := c.Add(contractABI)
	if err != nil {
		panic(err)
	}
}

func (c *ContractRegistry) MustRegister(contractName string, contractABI abi.ABI, contractBin []byte) Artifact {
	r, err := c.Register(contractName, contractABI, contractBin)
	if err != nil {
		panic(err)
	}
	return r
}

func (c *ContractRegistry) MustRegisterJSON(contractName string, contractABIJSON string, contractBin []byte) Artifact {
	r, err := c.RegisterJSON(contractName, contractABIJSON, contractBin)
	if err != nil {
		panic(err)
	}
	return r
}

func (c *ContractRegistry) MustGet(name string) Artifact {
	artifact, ok := c.Get(name)
	if !ok {
		panic(fmt.Sprintf("ethartifact: ContractRegistry#MustGet failed to get '%s'", name))
	}
	return artifact
}

func (c *ContractRegistry) ContractNames() []string {
	return c.names
}

func (c *ContractRegistry) Get(name string) (Artifact, bool) {
	artifact, ok := c.contracts[name]
	return artifact, ok
}

func (c *ContractRegistry) Encode(contractName, method string, args ...interface{}) ([]byte, error) {
	if c.contracts == nil {
		return nil, fmt.Errorf("contract registry cannot find contract %s", contractName)
	}
	artifact, ok := c.contracts[contractName]
	if !ok {
		return nil, fmt.Errorf("contract registry cannot find contract %s", contractName)
	}
	return artifact.ABI.Pack(method, args...)
}
