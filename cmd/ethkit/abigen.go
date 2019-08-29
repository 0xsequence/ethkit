package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/horizon-games/ethkit/ethartifacts"
	"github.com/spf13/cobra"
)

func init() {
	abigen := &abigen{}
	cmd := &cobra.Command{
		Use:   "abigen",
		Short: "generate contract interface from a truffle artifacts file",
		Run:   abigen.Run,
	}

	cmd.Flags().String("artifactsFile", "", "path to truffle contract artifacts file (required)")
	cmd.Flags().String("lang", "", "target language, supported: [go], default=go")
	cmd.Flags().String("pkg", "", "pkg (optional)")
	cmd.Flags().String("type", "", "type (optional)")
	cmd.Flags().String("outFile", "", "outFile (optional), default=stdout")

	rootCmd.AddCommand(cmd)
}

type abigen struct {
	fArtifactsFile string
	fPkg           string
	fType          string
	fOutFile       string
}

func (c *abigen) Run(cmd *cobra.Command, args []string) {
	c.fArtifactsFile, _ = cmd.Flags().GetString("artifactsFile")
	c.fPkg, _ = cmd.Flags().GetString("pkg")
	c.fType, _ = cmd.Flags().GetString("type")
	c.fOutFile, _ = cmd.Flags().GetString("outFile")

	if c.fArtifactsFile == "" {
		fmt.Println("error: please pass --artifactsFile")
		help(cmd)
		return
	}

	artifacts, err := ethartifacts.ParseArtifactsFile(c.fArtifactsFile)
	if err != nil {
		log.Fatal(err)
		return
	}

	if err := c.generateGo(artifacts); err != nil {
		log.Fatal(err)
		return
	}
}

func (c *abigen) generateGo(artifacts *ethartifacts.Artifacts) error {
	var (
		abis  []string
		bins  []string
		types []string
		sigs  []map[string]string
		libs  = make(map[string]string)
		lang  = bind.LangGo
	)

	if strings.Contains(string(artifacts.Bytecode), "//") {
		log.Fatal("Contract has additional library references, which is unsupported at this time.")
	}

	var pkgName string
	if c.fPkg != "" {
		pkgName = c.fPkg
	} else {
		pkgName = strings.ToLower(artifacts.ContractName)
	}

	var typeName string
	if c.fType != "" {
		typeName = c.fType
	} else {
		typeName = artifacts.ContractName
	}

	types = append(types, typeName)
	abis = append(abis, artifacts.ABI)
	bins = append(bins, artifacts.Bytecode)

	code, err := bind.Bind(types, abis, bins, sigs, pkgName, lang, libs)
	if err != nil {
		return err
	}

	if c.fOutFile == "" {
		fmt.Println(code)
	} else {
		if err := ioutil.WriteFile(c.fOutFile, []byte(code), 0600); err != nil {
			return err
		}
	}

	return nil
}
