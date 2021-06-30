package ethartifact

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
)

type Artifact struct {
	ContractName string
	ABI          abi.ABI
	Bin          []byte
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
	ContractName string          `json:"contractName"`
	ABI          json.RawMessage `json:"abi"`
	Bytecode     string          `json:"bytecode"`
}

func ParseArtifactFile(path string) (RawArtifact, error) {
	filedata, err := ioutil.ReadFile(path)
	if err != nil {
		return RawArtifact{}, err
	}

	var artifact RawArtifact
	err = json.Unmarshal(filedata, &artifact)
	if err != nil {
		return RawArtifact{}, err
	}

	return artifact, nil
}
