package ethartifact

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
)

type Artifact struct {
	ContractName string
	ABI          abi.ABI
	Bin          []byte
	DeployedBin  []byte
}

func (a Artifact) Encode(method string, args ...interface{}) ([]byte, error) {
	return a.ABI.Pack(method, args...)
}

func (a Artifact) Decode(result interface{}, method string, data []byte) error {
	return a.ABI.UnpackIntoInterface(result, method, data)
}

func ParseArtifactJSON(artifactJSON string) (Artifact, error) {
	var rawArtifact RawArtifact
	err := json.Unmarshal([]byte(artifactJSON), &rawArtifact)
	if err != nil {
		return Artifact{}, err
	}

	var artifact Artifact

	artifact.ContractName = rawArtifact.ContractName
	if rawArtifact.ContractName == "" {
		return Artifact{}, fmt.Errorf("contract name is empty")
	}

	parsedABI, err := abi.JSON(strings.NewReader(string(rawArtifact.ABI)))
	if err != nil {
		return Artifact{}, fmt.Errorf("unable to parse abi json in artifact: %w", err)
	}
	artifact.ABI = parsedABI

	if len(rawArtifact.Bytecode) > 2 {
		artifact.Bin = common.FromHex(rawArtifact.Bytecode)
	}
	if len(rawArtifact.DeployedBytecode) > 2 {
		artifact.DeployedBin = common.FromHex(rawArtifact.DeployedBytecode)
	}

	return artifact, nil
}

func MustParseArtifactJSON(artifactJSON string) Artifact {
	artifact, err := ParseArtifactJSON(artifactJSON)
	if err != nil {
		panic(err)
	}
	return artifact
}

type RawArtifact struct {
	ContractName     string          `json:"contractName"`
	ABI              json.RawMessage `json:"abi"`
	Bytecode         string          `json:"bytecode"`
	DeployedBytecode string          `json:"deployedBytecode"`
}

type FoundryRawArtifact struct {
	ABI      json.RawMessage `json:"abi"`
	Bytecode struct {
		Object string `json:"object"`
	} `json:"bytecode"`
	DeployedBytecode struct {
		Object string `json:"object"`
	} `json:"deployedBytecode"`
	Metadata struct {
		Settings struct {
			CompilationTarget map[string]string `json:"compilationTarget"`
		} `json:"settings"`
	} `json:"metadata"`
}

func ParseFoundryArtifactFile(path string) (FoundryRawArtifact, error) {
	filedata, err := os.ReadFile(path)
	if err != nil {
		return FoundryRawArtifact{}, err
	}

	var artifact FoundryRawArtifact
	err = json.Unmarshal(filedata, &artifact)
	if err != nil {
		return FoundryRawArtifact{}, err
	}

	return artifact, nil
}

func (f FoundryRawArtifact) ToRawArtifact() (RawArtifact, error) {
	// Contract name is the only value in the compilation target map
	if len(f.Metadata.Settings.CompilationTarget) != 1 {
		return RawArtifact{}, fmt.Errorf("expected exactly one compilation target, got %d", len(f.Metadata.Settings.CompilationTarget))
	}
	var contractName string
	for _, v := range f.Metadata.Settings.CompilationTarget {
		contractName = v
	}

	return RawArtifact{
		ContractName:     contractName,
		ABI:              f.ABI,
		Bytecode:         f.Bytecode.Object,
		DeployedBytecode: f.DeployedBytecode.Object,
	}, nil
}

func ParseArtifactFile(path string) (RawArtifact, error) {
	filedata, err := os.ReadFile(path)
	if err != nil {
		return RawArtifact{}, err
	}

	var artifact RawArtifact
	err = json.Unmarshal(filedata, &artifact)
	if err != nil {
		// Try parsing as foundry artifact
		foundryArtifact, foundryErr := ParseFoundryArtifactFile(path)
		if foundryErr != nil {
			// Return the original error
			return RawArtifact{}, err
		}
		return foundryArtifact.ToRawArtifact()
	}

	return artifact, nil
}
