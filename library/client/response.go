package client

import (
	"time"

	"github.com/seanhagen/workernator/library"
)

// jobResponse is returned from StartJob, StopJob, and JobStatus; it
// contains the information about a job such as the status and what
// command the job was asked to run. It fulfils library.JobInfo.
type jobResponse struct {
	id      string
	status  library.JobStatus
	cmd     string
	args    []string
	err     error
	started time.Time
	ended   time.Time
}

var _ library.JobInfo = &jobResponse{}

func (resp *jobResponse) ID() string {
	return resp.id
}

func (resp *jobResponse) Status() library.JobStatus {
	return resp.status
}

func (resp *jobResponse) Command() string {
	return resp.cmd
}

func (resp *jobResponse) Arguments() []string {
	return resp.args
}

func (resp *jobResponse) Error() error {
	return resp.err
}

func (resp *jobResponse) Started() time.Time {
	return resp.started
}

func (resp *jobResponse) Ended() time.Time {
	return resp.ended
}
