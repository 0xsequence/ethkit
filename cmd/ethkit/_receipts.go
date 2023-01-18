package main

import (
	"context"
	"fmt"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethreceipts"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/goware/logger"
	"github.com/spf13/cobra"
)

func init() {
	watch := &watch{}
	cmd := &cobra.Command{
		Use:   "receipts",
		Short: "Receipts ... etc..",
		Run:   watch.Run,
	}

	// cmd.Flags().String("file", "", "path to truffle contract artifacts file (required)")
	// cmd.Flags().Bool("abi", false, "abi")
	// cmd.Flags().Bool("bytecode", false, "bytecode")

	rootCmd.AddCommand(cmd)
}

type watch struct {
}

func (c *watch) Run(cmd *cobra.Command, args []string) {
	fmt.Println("xx")

	log := logger.NewLogger(logger.LogLevel_DEBUG)

	provider, err := ethrpc.NewProvider("https://xxx")
	if err != nil {
		panic(err)
	}

	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.Logger = log
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 1000

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	if err != nil {
		panic(err)
	}

	receipts, err := ethreceipts.NewReceiptsListener(log, provider, monitor)
	if err != nil {
		panic(err)
	}

	go func() {
		err := monitor.Run(context.Background())
		if err != nil {
			panic(err)
		}
	}()

	go func() {
		err := receipts.Run(context.Background())
		if err != nil {
			panic(err)
		}
	}()

	time.Sleep(10 * time.Minute)

	// fFile, _ := cmd.Flags().GetString("file")
	// fAbi, _ := cmd.Flags().GetBool("abi")
	// fBytecode, _ := cmd.Flags().GetBool("bytecode")

	// if fFile == "" {
	// 	fmt.Println("error: please pass --file")
	// 	help(cmd)
	// 	return
	// }
	// if !fAbi && !fBytecode {
	// 	fmt.Println("error: please pass either --abi or --bytecode")
	// 	help(cmd)
	// 	return
	// }
	// if fAbi && fBytecode {
	// 	fmt.Println("error: please pass either --abi or --bytecode, not both")
	// 	help(cmd)
	// 	return
	// }

	// artifacts, err := ethartifact.ParseArtifactFile(fFile)
	// if err != nil {
	// 	log.Fatal(err)
	// 	return
	// }

	// if fAbi {
	// 	fmt.Println(string(artifacts.ABI))
	// }

	// if fBytecode {
	// 	fmt.Println(artifacts.Bytecode)
	// }
}
