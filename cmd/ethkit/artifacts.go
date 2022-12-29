package main

import (
	"fmt"
	"log"

	"github.com/0xsequence/ethkit/ethartifact"
	"github.com/spf13/cobra"
)

func init() {
	artifacts := &artifacts{}
	cmd := &cobra.Command{
		Use:   "artifacts",
		Short: "Print the contract abi or bytecode from a truffle artifacts file",
		Run:   artifacts.Run,
	}

	cmd.Flags().String("file", "", "path to truffle contract artifacts file (required)")
	cmd.Flags().Bool("abi", false, "abi")
	cmd.Flags().Bool("bytecode", false, "bytecode")

	rootCmd.AddCommand(cmd)
}

type artifacts struct {
}

func (c *artifacts) Run(cmd *cobra.Command, args []string) {
	fFile, _ := cmd.Flags().GetString("file")
	fAbi, _ := cmd.Flags().GetBool("abi")
	fBytecode, _ := cmd.Flags().GetBool("bytecode")

	if fFile == "" {
		fmt.Println("error: please pass --file")
		help(cmd)
		return
	}
	if !fAbi && !fBytecode {
		fmt.Println("error: please pass either --abi or --bytecode")
		help(cmd)
		return
	}
	if fAbi && fBytecode {
		fmt.Println("error: please pass either --abi or --bytecode, not both")
		help(cmd)
		return
	}

	artifacts, err := ethartifact.ParseArtifactFile(fFile)
	if err != nil {
		log.Fatal(err)
		return
	}

	if fAbi {
		fmt.Println(string(artifacts.ABI))
	}

	if fBytecode {
		fmt.Println(artifacts.Bytecode)
	}
}
