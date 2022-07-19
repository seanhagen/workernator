package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/rs/xid"
	"github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library"
	"go.uber.org/zap"
)

const statusKilled = -1

// Config is used by NewManager to configure a Manager before returning it
type Config struct {
	// OutputPath is where the manager will store the final output of a job
	OutputPath string
}

// Manager is what handles ( ie, manages ) running jobs, including:
//  - starting jobs
//  - stopping jobs
//  - getting the status of a running job
//  - getting the output of a job
//
// It should be initialized using NewManager(conf Config).
type Manager struct {
	jobs    map[xid.ID]*job
	jobLock sync.Mutex

	outputDir string
}

// NewManager builds a new Manager, and sets up any necessary directories
func NewManager(conf Config) (*Manager, error) {
	err := os.MkdirAll(conf.OutputPath, 0755)
	if err != nil {
		return nil, fmt.Errorf("unable to set up directory: %w", err)
	}

	manager := &Manager{
		jobs:      map[xid.ID]*job{},
		outputDir: conf.OutputPath,
		jobLock:   sync.Mutex{},
	}

	return manager, nil
}

// StartJob builds and starts the command as a running job, returning
// an object that provides some information about the job, as well as
// the ability to wait for the job to finish, or to stop the job early
// by killing it.
func (m *Manager) StartJob(ctx context.Context, command string, args ...string) (library.Job, error) {
	id := xid.New()

	jobOutputDir := m.outputDir + "/" + id.String()
	if err := os.MkdirAll(jobOutputDir, 0755); err != nil {
		return nil, fmt.Errorf("unable to create job output directory: %w", err)
	}

	stdoutFile, err := os.OpenFile(jobOutputDir+"/output", os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_SYNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("unable to create file to capture output: %w", err)
	}

	stderrFile, err := os.OpenFile(jobOutputDir+"/error", os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_SYNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("unable to create file to capture errors: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)

	cmd := exec.Command(command, args...)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	j := &job{
		id:     id,
		status: pb.JobStatus_Running,

		command: command,
		args:    args,

		cmd:  cmd,
		done: ctx,

		stdout: stdoutFile,
	}

	m.jobLock.Lock()
	m.jobs[id] = j
	m.jobLock.Unlock()

	go func() {
		err := cmd.Wait()
		st := cmd.ProcessState.ExitCode()

		m.jobLock.Lock()
		j := m.jobs[id]

		j.status = pb.JobStatus_Finished

		if x := stdoutFile.Close(); x != nil {
			zap.L().Error("unable to close output file", zap.Error(err))
		}
		if x := stderrFile.Close(); x != nil {
			zap.L().Error("unable to close error output file", zap.Error(err))
		}

		if err != nil {
			j.err = err
			j.status = pb.JobStatus_Failed
		}

		if err == nil && st != 0 {
			j.err = fmt.Errorf("exited with status %v", st)
			j.status = pb.JobStatus_Failed
		}

		if st == statusKilled {
			j.status = pb.JobStatus_Stopped
		}

		m.jobs[id] = j
		m.jobLock.Unlock()
		cancel()
	}()

	return j, nil
}

// JobStatus returns the status of a job, whether it's running or
// already finished. If the ID provided is either not a valid xid or
// not the ID of a job an error will be returned.
func (m *Manager) JobStatus(ctx context.Context, id string) (library.JobInfo, error) {
	job, err := m.validateID(id)
	if err != nil {
		return nil, err
	}

	return job, nil
}

// StopJob attempts to stop the job indicated by the ID. If the ID
// provided is either not a valid xid or not the ID of a job an error
// will be returned.
func (m *Manager) StopJob(ctx context.Context, id string) (library.JobInfo, error) {
	job, err := m.validateID(id)
	if err != nil {
		return nil, err
	}

	if err := job.Stop(); err != nil {
		return nil, err
	}
	return job, nil
}

// GetJobOutput returns an io.Reader that can be read to get the
// output of a job. If the ID provided is either not a valid xid or
// not the ID of a job an error will be returned.
func (m *Manager) GetJobOutput(ctx context.Context, id string) (io.Reader, error) {
	job, err := m.validateID(id)
	if err != nil {
		return nil, err
	}
	// this is the cool bit
	read, write := io.Pipe()

	// path to our output file
	path := m.outputDir + "/" + job.ID() + "/output"
	output, err := os.OpenFile(path, os.O_RDONLY, 0444)
	if err != nil {
		zap.L().Error("unable to open output file", zap.Error(err))
		return nil, err
	}

	// launch a go routine to read from the file and write to the pipe
	go pipeToOutput(output, job, write)

	// immediately return the pipe reader for the user
	return read, nil
}

// validateID validates that the ID given is a valid xid, and the ID
// of a job started by this manager.
func (m *Manager) validateID(id string) (*job, error) {
	jid, err := xid.FromString(id)
	if err != nil {
		return nil, library.NewErrInvalidID(id, err)
	}

	job, ok := m.jobs[jid]
	if !ok {
		return nil, library.NewErrNoJobForID(id)
	}

	return job, nil
}

// pipeToOutput is meant to be launched as a goroutine so that it can
// read from the provided file and write to the io pipe provided. If
// the job hasn't finished, it will wait until it has before
// exiting. This provides the 'tail' functionality.
func pipeToOutput(file *os.File, cmdJob *job, writeTo *io.PipeWriter) {
	var lastSize int64
	buf := make([]byte, 1024)

	for {
		fi, err := file.Stat()
		if err != nil {
			x := fmt.Errorf("unable to stat file: %w", err)
			_ = writeTo.CloseWithError(x)
			return
		}

		if fi.Size() > lastSize {
			n, err := file.Read(buf)

			// if we read anything, first write it to our pipe
			if n > 0 {
				lastSize += int64(n)
				_, err = writeTo.Write(buf[:n])
				if err != nil {
					_ = writeTo.CloseWithError(err)
					return
				}
			}

			if err == nil && cmdJob.Finished() {
				_ = writeTo.CloseWithError(io.EOF)
				return
			}

			// unable to read from the file because we've reached the end?
			if errors.Is(err, io.EOF) {
				if cmdJob.Finished() {
					_ = writeTo.CloseWithError(io.EOF)
					return
				}
				goto wait
			}

			if err != nil {
				_ = writeTo.CloseWithError(err)
			}
		}

	wait:
		time.Sleep(time.Millisecond * 200)
	}
}
