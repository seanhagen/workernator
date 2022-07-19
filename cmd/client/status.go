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
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/rs/xid"
	"github.com/seanhagen/workernator/library"
	"github.com/spf13/cobra"
)

var statusTemplate string = `Job Status:
ID:     {{.ID}}
Status: {{.Status}}

Command run: {{.Command}}
Arguments: {{.Arguments}}

Started: {{.Started}}
{{- if ne .Ended ""}}
Ended: {{.Ended}}
{{- end}}
{{- if ne .Error ""}}
Error: {{.Error}}
{{- end}}
`

var statusT = template.Must(template.New("status").Parse(statusTemplate))

type statusTemplateData struct {
	ID        string
	Status    string
	Command   string
	Arguments string
	Started   string
	Ended     string
	Error     string
}

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get the status of a job",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,

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

		resp, err := apiClient.JobStatus(ctx, args[0])
		if err != nil {
			return fmt.Errorf("unable to get job status: %w", err)
		}

		data := statusToTemplateData(resp)
		err = statusT.Execute(os.Stdout, data)
		if err != nil {
			return fmt.Errorf("unable to render status: %w", err)
		}

		return nil
	},
}

func statusToTemplateData(resp library.JobInfo) statusTemplateData {
	out := statusTemplateData{
		ID:        resp.ID(),
		Command:   resp.Command(),
		Arguments: strings.Join(resp.Arguments(), ", "),
		Started:   resp.Started().Format(time.RFC3339),
	}

	switch resp.Status() {
	case library.Running:
		out.Status = "Running"
	case library.Failed:
		out.Status = "Failed"
	case library.Finished:
		out.Status = "Finished"
	case library.Stopped:
		out.Status = "Stopped"
	case library.Unknown:
		fallthrough
	default:
		out.Status = "Unknown"
	}

	if !resp.Ended().IsZero() {
		out.Ended = resp.Ended().Format(time.RFC3339)
	}

	if resp.Error() != nil {
		out.Error = resp.Error().Error()
	}
	return out
}

func init() {
	jobsCmd.AddCommand(statusCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// statusCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// statusCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
