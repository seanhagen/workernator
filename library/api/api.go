package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/rs/xid"
	"github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library"
)

const statusKilled = -1

// Config ...
type Config struct {
	// TmpPath is where the manager will create & store temporary files
	// related to running commands, or preparing images
	TmpPath string

	// OutputPath is where the manager will store the final output of a job
	OutputPath string
}

// Manager ...
type Manager struct {
	jobs    map[xid.ID]*job
	jobLock sync.Mutex

	tmpDir string
}

// NewManager ...
func NewManager(pathToTempRoot string) (*Manager, error) {
	tmp, err := os.MkdirTemp(pathToTempRoot, "workernator-manager")
	if err != nil {
		return nil, fmt.Errorf("unable to set up temporary directory: %w", err)
	}

	manager := &Manager{
		jobs:    map[xid.ID]*job{},
		tmpDir:  tmp,
		jobLock: sync.Mutex{},
	}

	return manager, nil
}

// StartJob  ...
func (m *Manager) StartJob(ctx context.Context, command string, args ...string) (library.Job, error) {
	id := xid.New()

	jobOutputDir := m.tmpDir + "/" + id.String()
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

		//fmt.Printf("##############################\nJOB COMPLETE\n##############################\n\n")

		m.jobLock.Lock()
		j := m.jobs[id]

		j.status = pb.JobStatus_Finished

		if x := stdoutFile.Close(); x != nil {
			fmt.Printf("unable to close output file: %v\n", err)
		}
		if x := stderrFile.Close(); x != nil {
			fmt.Printf("unable to close error output file: %v\n", err)
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

// JobStatus  ...
func (m *Manager) JobStatus(ctx context.Context, id string) (library.JobInfo, error) {
	job, err := m.validateID(id)
	if err != nil {
		return nil, err
	}

	return job, nil
}

// StopJob  ...
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

// GetJobOutput  ...
func (m *Manager) GetJobOutput(ctx context.Context, id string) (io.Reader, error) {
	job, err := m.validateID(id)
	if err != nil {
		return nil, err
	}
	//fmt.Printf("job %v is valid\n", id)

	path := m.tmpDir + "/" + job.ID() + "/output"
	//fmt.Printf("reading output from '%v'\n", path)

	read, write := io.Pipe()
	f, err := os.OpenFile(path, os.O_RDONLY, 0444)
	if err != nil {
		fmt.Printf("unable to open output file: %v\n", err)
		return nil, err
	}

	//fmt.Printf("launching goroutine to read from file and write to pipe!\n")
	go pipeToOutput(f, job, read, write)

	return read, nil
}

// validateID ...
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

func pipeToOutput(file *os.File, cmdJob *job, clientReader *io.PipeReader, writeTo *io.PipeWriter) {
	//fmt.Printf("[pto] pipe to output, started!\n")
	var lastSize int64
	buf := make([]byte, 1024)

	for {
		fi, err := file.Stat()
		if err != nil {
			//fmt.Printf("[pto] unable to stat file: %v\n", err)
			x := fmt.Errorf("unable to stat file: %w", err)
			// clientReader.CloseWithError(x)
			writeTo.CloseWithError(x)
			return
		}

		//fmt.Printf("[pto] file size: %v, last size: %v\n", fi.Size(), lastSize)
		if fi.Size() > lastSize {
			n, err := file.Read(buf)

			// if we read anything, first write it to our pipe
			if n > 0 {
				lastSize += int64(n)
				//fmt.Printf("[pto] read %v bytes, writing to pipe...", n)
				// m, x := writeTo.Write(buf[:n])
				//fmt.Printf(" wrote %v bytes (error: %v)\n", m, x)
				writeTo.Write(buf[:n])
			}
			//else {
			//fmt.Printf("[pto] didn't read anything from the file?\n")
			//}

			if err == nil && cmdJob.Finished() {
				//fmt.Printf("[pto] job done!\n")
				// clientReader.CloseWithError(io.EOF)
				writeTo.CloseWithError(io.EOF)
				return
			}

			//fmt.Printf("[pto] encountered error: %v\n", err)

			// unable to read from the file because we've reached the end?
			if err == io.EOF {
				//fmt.Printf("[pto] reached end of file!\n")
				// if the command has finished running, then we're really done for reals
				if cmdJob.Finished() {
					//fmt.Printf("[pto] job is finished!\n")
					// clientReader.CloseWithError(io.EOF)
					writeTo.CloseWithError(io.EOF)
					return
				}
				//fmt.Printf("[pto] job still running, waiting 200ms\n")
				// if the command isn't finished, wait a bit to see if we get more output or the job finishes
				goto wait
			}

			if err != nil {
				//fmt.Printf("[pto] encountered non-EOF error: %v\n", err)
				writeTo.CloseWithError(err)
			}
		}
		//fmt.Printf("[pto] no change in size\n")

	wait:
		time.Sleep(time.Millisecond * 200)
	}
}
