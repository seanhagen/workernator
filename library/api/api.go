package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/rs/xid"
	"github.com/seanhagen/workernator/library"
)

// Config is used by NewManager to configure a Manager before
// returning it
type Config struct {
	// OutputPath is where the manager will store the final output of a
	// job
	OutputPath string
}

// Manager is what handles ( ie, manages ) jobs, including:
//  - starting jobs
//  - stopping jobs
//  - getting the status of a running job
//  - getting the output of a job
//
// It should be initialized using NewManager(conf Config).
type Manager struct {
	jobs    map[string]*library.Job
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
		jobs:      map[string]*library.Job{},
		outputDir: conf.OutputPath,
	}

	return manager, nil
}

// StartJob builds and starts the command as a running job, returning
// an object that provides some information about the job, as well as
// the ability to wait for the job to finish, or to stop the job early
// by killing it.
func (m *Manager) StartJob(ctx context.Context, command string, args ...string) (*library.Job, error) {
	job, err := library.NewJob(ctx, m.outputDir, command, args...)
	if err != nil {
		return nil, fmt.Errorf("unable to create job: %w", err)
	}

	m.jobLock.Lock()
	m.jobs[job.ID] = job
	m.jobLock.Unlock()

	return job, nil
}

// JobStatus returns the status of a job, whether it's running or
// already finished. If the ID provided is either not a valid xid or
// not the ID of a job an error will be returned.
func (m *Manager) JobStatus(ctx context.Context, id string) (*library.JobInfo, error) {
	job, err := m.validateID(id)
	if err != nil {
		return nil, err
	}

	info := job.Info()
	return &info, nil
}

// StopJob attempts to stop the job indicated by the ID. If the ID
// provided is either not a valid xid or not the ID of a job an error
// will be returned.
func (m *Manager) StopJob(ctx context.Context, id string) (*library.JobInfo, error) {
	job, err := m.validateID(id)
	if err != nil {
		return nil, err
	}

	if err := job.Stop(); err != nil {
		return nil, err
	}
	info := job.Info()
	return &info, nil
}

// GetJobOutput returns an io.Reader that can be read to get the
// output of a job. If the ID provided is either not a valid xid or
// not the ID of a job an error will be returned.
func (m *Manager) GetJobOutput(ctx context.Context, id string) (io.ReadCloser, error) {
	job, err := m.validateID(id)
	if err != nil {
		return nil, err
	}

	return job.GetOutput()
}

// validateID validates that the ID given is a valid xid, and the ID
// of a job started by this manager.
func (m *Manager) validateID(id string) (*library.Job, error) {
	if _, err := xid.FromString(id); err != nil {
		return nil, library.NewErrInvalidID(id, err)
	}

	job, ok := m.jobs[id]
	if !ok {
		return nil, library.NewErrNoJobForID(id)
	}

	return job, nil
}
