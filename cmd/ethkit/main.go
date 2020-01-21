package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	VERSION       = "dev"
	GITBRANCH     = "branch"
	GITCOMMIT     = "last commit"
	GITCOMMITDATE = "last change"
)

var rootCmd = &cobra.Command{
	Use:   "ethkit",
	Short: "ETHKIT - Ethereum wallet, client & dev toolkit",
	Args:  cobra.MinimumNArgs(1),
}

func init() {
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("ethkit", version())
		},
	}

	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func help(cmd *cobra.Command) {
	fmt.Printf("\n--\n\n")
	cmd.Help()
	os.Exit(0)
}

func version() string {
	if GITBRANCH == "master" {
		return fmt.Sprintf("%s (%s %s)", VERSION, GITCOMMIT, GITCOMMITDATE)
	}
	return fmt.Sprintf("%s (%s:%s %s)", VERSION, GITBRANCH, GITCOMMIT, GITCOMMITDATE)
}
