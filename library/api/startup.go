package api

import (
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
)

const (
	// WorkernatorServerCmdName should be used when creating the root command
	WorkernatorServerCmdName = "workernator"

	startingInNamespace = "__WORKERNATOR__NAMESPACED__"
	finalRun            = "__WORKERNATOR__FINAL__"
)

// RunInNamespaceCmd generates the cobra command the server uses when
// launching a job in a container. This should be called in init() in the main package, and
// the returned command should be added to the root command
func RunInNamespaceCmd() *cobra.Command {
	var flagTest string

	cmd := &cobra.Command{
		Use:    startingInNamespace,
		Short:  "special command, do not use",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			spew.Dump(args, cmd.Flag("flag"), flagTest)
			fmt.Fprintf(os.Stdout, "this is where the command gets all set up so it's ready to run\n")
			return nil
		},
	}

	cmd.Flags().StringVarP(&flagTest, "flag", "f", "", "just testing")

	return cmd
}

// LaunchJobCmd handles the the last bits of setup before running the actual
// 'job' command the user has requested.
func LaunchJobCmd() *cobra.Command {
	return &cobra.Command{
		Use:    finalRun,
		Short:  "special command, do not use",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			spew.Dump(args)
			fmt.Fprintf(os.Stdout, "this is where some final setup happens, and then the /actual/ job command gets run\n")
			return nil
		},
	}
}
