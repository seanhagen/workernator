package main

import (
	"context"
	"fmt"

	"github.com/seanhagen/workernator/library/client"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(rootCmd.Execute())
}

var (
	host      string
	port      string
	certPath  string
	keyPath   string
	chainPath string
)

var (
	apiClient *client.Client
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "client",
	Short: "A client for interacting with a running workernator server",
	Long: `Workernator is a job-runner library, server, and CLI client used for
long-running tasks you don't want to run as part of your core service.

This is the CLI client application, which allows you to start jobs,
stop jobs, get the status of jobs, and tail the output of any job.`,
}

// jobsCmd represents the jobs command
var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "Sub-command for interacting with jobs",
	Long: `This sub-command provides the ability to manage jobs, including
starting, stopping, getting the status, and viewing the output.`,
	TraverseChildren: true,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		conf := client.Config{
			Host:      host,
			Port:      port,
			CertPath:  certPath,
			KeyPath:   keyPath,
			ChainPath: chainPath,
		}

		c, err := client.NewClient(ctx, conf)
		if err != nil {
			return fmt.Errorf("unable to create API client: %w", err)
		}

		apiClient = c
		return nil
	},
}

func init() {
	jobsCmd.PersistentFlags().StringVarP(
		&host, "host", "H", "",
		"host of the server to connect to, eg 'localhost' or '127.0.0.1'",
	)

	jobsCmd.PersistentFlags().StringVarP(
		&port, "port", "P", "",
		"port on the server to connect to, should be between 1 and 65,353",
	)

	jobsCmd.PersistentFlags().StringVarP(
		&certPath, "certPath", "c", "",
		"path to the client mTLS certificate to use for authentication",
	)

	jobsCmd.PersistentFlags().StringVarP(
		&keyPath, "keyPath", "k", "",
		"path to the key file used when generating the certificates",
	)

	jobsCmd.PersistentFlags().StringVarP(
		&chainPath, "chainPath", "a", "",
		"path to the chain pem file used to sign the server certificates",
	)

	rootCmd.AddCommand(jobsCmd)
}
