package server

import (
	"context"
	"io"
	"testing"

	"github.com/seanhagen/workernator/library"
	"github.com/stretchr/testify/require"
)

func TestWorker_NewService(t *testing.T) {
	var svc *Service
	var err error

	mgr := testManager{}

	svc, err = NewService(mgr)
	require.NoError(t, err)
	require.NotNil(t, svc)
}

func TestWorker_Service_Start(t *testing.T) {

}

type testManager struct {
	startFn  func(ctx context.Context, cmd string, args ...string) (*library.Job, error)
	stopFn   func(ctx context.Context, id string) (*library.JobInfo, error)
	statusFn func(ctx context.Context, id string) (*library.JobInfo, error)
	outputFn func(ctx context.Context, id string) (io.Reader, error)
}

var _ Manager = testManager{}

// StartJob  ...
func (tm testManager) StartJob(ctx context.Context, cmd string, args ...string) (*library.Job, error) {
	return tm.startFn(ctx, cmd, args...)
}

// StopJob  ...
func (tm testManager) StopJob(ctx context.Context, id string) (*library.JobInfo, error) {
	return tm.stopFn(ctx, id)
}

func (tm testManager) JobStatus(ctx context.Context, id string) (*library.JobInfo, error) {
	return tm.statusFn(ctx, id)
}

func (tm testManager) GetJobOutput(ctx context.Context, id string) (io.Reader, error) {
	return tm.outputFn(ctx, id)
}
