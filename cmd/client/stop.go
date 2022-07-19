/*
Copyright © 2022 Sean Patrick Hagen <sean.hagen@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package main

import (
	"context"
	"fmt"

	"github.com/rs/xid"
	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop [id]",
	Short: "Stop a running job",
	Long: `Asks the server to stop the job belonging to the ID provided.

Doesn't do anything if the job isn't running, returns successfully.

If the job doesn't exist, will return an error.`,

	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) <= 0 {
			return fmt.Errorf("id argument is required")
		}

		_, err := xid.FromString(args[0])
		if err != nil {
			return fmt.Errorf("first argument must be valid xid")
		}

		return nil
	},

	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		resp, err := apiClient.StopJob(ctx, args[0])
		if err != nil {
			return fmt.Errorf("unable to stop job: %w", err)
		}

		fmt.Printf("stopped job '%v'\n", args[0])

		return nil
	},
}

func init() {
	jobsCmd.AddCommand(stopCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// stopCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// stopCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
