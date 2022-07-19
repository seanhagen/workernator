package api

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/rs/xid"
	"github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library"
)

// job fulfills the library.Job interface ( and so fulfils the
// library.JobInfo interface as well )
type job struct {
	id     xid.ID
	status pb.JobStatus

	command string
	args    []string
	started time.Time
	ended   time.Time

	cmd  *exec.Cmd
	done context.Context
	err  error

	stdout *os.File
}

func (ji *job) Wait() error {
	<-ji.done.Done()
	return ji.err
}

func (ji job) ID() string {
	return ji.id.String()
}

func (ji job) Status() library.JobStatus {
	return library.JobStatus(ji.status.Number())
}

func (ji job) Command() string {
	return ji.command
}

func (ji job) Arguments() []string {
	return ji.args
}

func (ji job) Error() error {
	return ji.err
}

// Started  ...
func (ji job) Started() time.Time {
	return ji.started
}

// Ended  ...
func (ji job) Ended() time.Time {
	return ji.ended
}

func (ji job) Finished() bool {
	return ji.status == pb.JobStatus_Finished ||
		ji.status == pb.JobStatus_Failed ||
		ji.status == pb.JobStatus_Stopped
}

func (ji *job) Stop() error {
	err := ji.cmd.Process.Kill()
	<-ji.done.Done()
	if err != nil {
		return err
	}
	return nil
}
