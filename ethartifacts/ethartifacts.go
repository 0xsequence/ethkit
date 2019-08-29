package ethartifacts

import (
	"encoding/json"
	"io/ioutil"

	"github.com/pkg/errors"
)

type Artifacts struct {
	ContractName string
	ABI          string
	Bytecode     string
}

func ParseArtifactsFile(path string) (*Artifacts, error) {
	filedata, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var parsed map[string]interface{}
	err = json.Unmarshal(filedata, &parsed)
	if err != nil {
		return nil, err
	}

	var ok bool
	artifacts := &Artifacts{}

	artifacts.ContractName, ok = parsed["contractName"].(string)
	if !ok {
		return nil, errors.Errorf("parsed artifacts file contains invalid 'contractName' field")
	}

	abiJSON, err := json.Marshal(parsed["abi"])
	if err != nil {
		return nil, errors.Wrapf(err, "parsed artifacts file contains invalid 'abi' field")
	}

	artifacts.ABI = string(abiJSON)

	artifacts.Bytecode, ok = parsed["bytecode"].(string)
	if !ok || artifacts.Bytecode == "" {
		return nil, errors.Errorf("parsed artifacts file contains invalid 'bytecode' field")
	}

	return artifacts, nil
}
