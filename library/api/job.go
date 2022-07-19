package api

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/rs/xid"
	"github.com/seanhagen/workernator/internal/pb"
)

type jobOutput struct {
	file *os.File
	buf  *bytes.Buffer
}

// job fulfills the library.Job interface ( and so fulfils the
// library.JobInfo interface as well )
type job struct {
	id     xid.ID
	status pb.JobStatus

	command string
	args    []string

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

func (ji job) Status() string {
	return ji.status.String()
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

func (ji job) Finished() bool {
	return ji.status == pb.JobStatus_Finished ||
		ji.status == pb.JobStatus_Failed ||
		ji.status == pb.JobStatus_Stopped
}

func (ji *job) Stop() error {
	err := ji.cmd.Process.Kill()
	<-ji.done.Done()
	if err != nil {
		ji.err = err
		return err
	}
	return nil
}
