package ethartifact

import (
	"os"
	"strings"
	"testing"
)

func TestParseArtifactFile_Hardhat(t *testing.T) {
	hardhatPath := "testdata/hardhat_artifact.json"
	if _, err := os.Stat(hardhatPath); err != nil {
		t.Skipf("Hardhat artifact file not found: %v", err)
	}

	artifact, err := ParseArtifactFile(hardhatPath)
	if err != nil {
		t.Fatalf("ParseArtifactFile failed for Hardhat artifact: %v", err)
	}

	// Check contract name
	if artifact.ContractName != "ERC20" {
		t.Errorf("Expected contract name 'ERC20', got '%s'", artifact.ContractName)
	}

	// Check ABI is not empty
	if len(artifact.ABI) == 0 {
		t.Error("Expected ABI to be parsed, but it's empty")
	}

	// Check bytecode is not empty and has 0x prefix
	if len(artifact.Bytecode) == 0 {
		t.Error("Expected bytecode to be parsed, but it's empty")
	}
	if !strings.HasPrefix(artifact.Bytecode, "0x") {
		t.Error("Expected bytecode to start with '0x'")
	}

	// Check deployed bytecode is not empty and has 0x prefix
	if len(artifact.DeployedBytecode) == 0 {
		t.Error("Expected deployed bytecode to be parsed, but it's empty")
	}
	if !strings.HasPrefix(artifact.DeployedBytecode, "0x") {
		t.Error("Expected deployed bytecode to start with '0x'")
	}
}

func TestParseArtifactFile_Foundry(t *testing.T) {
	foundryPath := "testdata/foundry_artifact.json"
	if _, err := os.Stat(foundryPath); err != nil {
		t.Skipf("Foundry artifact file not found: %v", err)
	}

	artifact, err := ParseArtifactFile(foundryPath)
	if err != nil {
		t.Fatalf("ParseArtifactFile failed for Foundry artifact: %v", err)
	}

	// Check contract name
	if artifact.ContractName != "ValueForwarder" {
		t.Errorf("Expected contract name 'ValueForwarder', got '%s'", artifact.ContractName)
	}

	// Check ABI is not empty
	if len(artifact.ABI) == 0 {
		t.Error("Expected ABI to be parsed, but it's empty")
	}

	// Check bytecode is not empty and has 0x prefix
	if len(artifact.Bytecode) == 0 {
		t.Error("Expected bytecode to be parsed, but it's empty")
	}
	if !strings.HasPrefix(artifact.Bytecode, "0x") {
		t.Error("Expected bytecode to start with '0x'")
	}

	// Check deployed bytecode is not empty and has 0x prefix
	if len(artifact.DeployedBytecode) == 0 {
		t.Error("Expected deployed bytecode to be parsed, but it's empty")
	}
	if !strings.HasPrefix(artifact.DeployedBytecode, "0x") {
		t.Error("Expected deployed bytecode to start with '0x'")
	}
}
