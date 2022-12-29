package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/0xsequence/ethkit/ethartifact"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
	"github.com/spf13/cobra"
)

func init() {
	abigen := &abigen{}
	cmd := &cobra.Command{
		Use:   "abigen",
		Short: "Generate contract Go client code from an abi or truffle artifacts file",
		Run:   abigen.Run,
	}

	cmd.Flags().String("artifactsFile", "", "path to truffle contract artifacts file")
	cmd.Flags().String("abiFile", "", "path to abi json file")
	cmd.Flags().String("lang", "", "target language, supported: [go], default=go")
	cmd.Flags().String("pkg", "", "pkg (optional)")
	cmd.Flags().String("type", "", "type (optional)")
	cmd.Flags().String("outFile", "", "outFile (optional), default=stdout")
	cmd.Flags().Bool("includeDeployed", false, "include deployed bytecode on the generated file")

	rootCmd.AddCommand(cmd)
}

type abigen struct {
	fArtifactsFile   string
	fAbiFile         string
	fPkg             string
	fType            string
	fOutFile         string
	fIncludeDeployed bool
}

func (c *abigen) Run(cmd *cobra.Command, args []string) {
	c.fArtifactsFile, _ = cmd.Flags().GetString("artifactsFile")
	c.fAbiFile, _ = cmd.Flags().GetString("abiFile")
	c.fPkg, _ = cmd.Flags().GetString("pkg")
	c.fType, _ = cmd.Flags().GetString("type")
	c.fOutFile, _ = cmd.Flags().GetString("outFile")
	c.fIncludeDeployed, _ = cmd.Flags().GetBool("includeDeployed")

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
		dbins []string
		types []string
		sigs  []map[string]string
		libs  = make(map[string]string)
		lang  = bind.LangGo
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

	if c.fIncludeDeployed {
		dbins = append(dbins, artifact.DeployedBytecode)

		if strings.Contains(string(artifact.DeployedBytecode), "//") {
			log.Fatal("Contract has additional library references, which is unsupported at this time.")
		}
	} else {
		dbins = append(dbins, "")
	}

	code, err := bind.Bind(types, abis, bins, dbins, sigs, pkgName, lang, libs, aliases)
	if err != nil {
		return err
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
