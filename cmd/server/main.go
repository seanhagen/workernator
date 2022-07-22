package main

import (
	"github.com/seanhagen/workernator/library/api"
	"github.com/seanhagen/workernator/library/container"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(rootCmd.Execute())
}

var rootCmd = &cobra.Command{
	Use:   api.WorkernatorServerCmdName,
	Short: "The GRPC server for workernator",
}

func init() {
	rootCmd.AddCommand(container.RunContainer())
	rootCmd.AddCommand(container.RunInNamespaceCmd())
	rootCmd.AddCommand(container.SetupNetNS())
	rootCmd.AddCommand(container.SetupVeth())
}
