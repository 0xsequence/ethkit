package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/0xsequence/ethkit/ethartifact"
	geth_abigen "github.com/0xsequence/ethkit/go-ethereum/accounts/abi/abigen"
	"github.com/spf13/cobra"
)

func init() {
	abigen := &abigen{}
	cmd := &cobra.Command{
		Use:   "abigen",
		Short: "Generate contract Go client code from an abi or contract artifact file",
		Run:   abigen.Run,
	}

	cmd.Flags().String("artifactsFile", "", "path to compiled contract artifact file")
	cmd.Flags().String("abiFile", "", "path to abi json file (optional)")
	cmd.Flags().String("pkg", "", "go package name")
	cmd.Flags().String("type", "", "type name used in generated output")
	cmd.Flags().String("outFile", "", "outFile (optional, default=stdout)")
	cmd.Flags().Bool("includeDeployedBin", false, "include deployed bytecode in the generated file (default=false)")
	cmd.Flags().Bool("v2", false, "use go-ethereum abigen v2 (default=false)")

	rootCmd.AddCommand(cmd)
}

type abigen struct {
	fArtifactsFile      string
	fAbiFile            string
	fPkg                string
	fType               string
	fOutFile            string
	fIncludeDeployedBin bool
	fUseV2              bool
}

func (c *abigen) Run(cmd *cobra.Command, args []string) {
	c.fArtifactsFile, _ = cmd.Flags().GetString("artifactsFile")
	c.fAbiFile, _ = cmd.Flags().GetString("abiFile")
	c.fPkg, _ = cmd.Flags().GetString("pkg")
	c.fType, _ = cmd.Flags().GetString("type")
	c.fOutFile, _ = cmd.Flags().GetString("outFile")
	c.fIncludeDeployedBin, _ = cmd.Flags().GetBool("includeDeployedBin")
	c.fUseV2, _ = cmd.Flags().GetBool("v2")

	if c.fArtifactsFile == "" && c.fAbiFile == "" {
		fmt.Println("error: please pass one of --artifactsFile or --abiFile")
		help(cmd)
		return
	}

	if c.fAbiFile != "" && c.fPkg == "" {
		fmt.Println("error: please pass --pkg")
		help(cmd)
		return
	}
	if c.fAbiFile != "" && c.fType == "" {
		fmt.Println("error: please pass --pkg")
		help(cmd)
		return
	}

	var artifact ethartifact.RawArtifact
	var err error

	if c.fArtifactsFile != "" {
		artifact, err = ethartifact.ParseArtifactFile(c.fArtifactsFile)
		if err != nil {
			log.Fatal(err)
			return
		}
	} else {
		abiData, err := os.ReadFile(c.fAbiFile)
		if err != nil {
			log.Fatal(err)
			return
		}
		artifact = ethartifact.RawArtifact{ABI: abiData}
	}

	if err := c.generateGo(artifact); err != nil {
		log.Fatal(err)
		return
	}
}

func (c *abigen) generateGo(artifact ethartifact.RawArtifact) error {
	var (
		abis  []string
		bins  []string
		types []string
		sigs  []map[string]string
		libs  = make(map[string]string)
	)

	if strings.Contains(string(artifact.Bytecode), "//") {
		log.Fatal("Contract has additional library references, which is unsupported at this time.")
	}

	var pkgName string
	if c.fPkg != "" {
		pkgName = c.fPkg
	} else {
		pkgName = strings.ToLower(artifact.ContractName)
	}

	var typeName string
	if c.fType != "" {
		typeName = c.fType
	} else {
		typeName = artifact.ContractName
	}

	types = append(types, typeName)
	abis = append(abis, string(artifact.ABI))
	bins = append(bins, artifact.Bytecode)
	aliases := map[string]string{}

	var code string
	var err error

	// NOTE: "bytecode" in an artifact contains both the constructor + the runtime code of the contract,
	// the "bytecode" value is what we use to deploy a new contract.
	//
	// Whereas the "deployedBytecode" is the runtime code of the contract, and can be used to verify
	// the contract bytecode once its been deployed. For our purposes of generating a client, we only
	// need the constructor code, so we use the "bytecode" value.

	if c.fUseV2 {
		code, err = geth_abigen.BindV2(types, abis, bins, pkgName, libs, aliases)
	} else {
		code, err = geth_abigen.Bind(types, abis, bins, sigs, pkgName, libs, aliases)
	}
	if err != nil {
		return err
	}

	if c.fIncludeDeployedBin {
		code = fmt.Sprintf("%s\n// %sDeployedBin is the resulting bytecode of the created contract\nconst %sDeployedBin = %q\n", code, typeName, typeName, artifact.DeployedBytecode)
	}

	if c.fOutFile == "" {
		fmt.Println(code)
	} else {
		if err := os.WriteFile(c.fOutFile, []byte(code), 0600); err != nil {
			return err
		}
	}

	return nil
}
