package library

import (
	"fmt"
)

const (
	// this is a potential status code returned by
	// (*exec.Cmd).ProcessState.ExitCode(); it's -1 when a job is still
	// running or when it was terminated via a signal
	statusKilled = -1

	// this is the status code returned in UNIX when a program exits
	// successfully
	statusOK = 0
)

// JobStatus is used to define the potential statuses for a job
type JobStatus int

const (
	// Unknown is the default value, and should be treated as an error
	Unknown JobStatus = iota
	// Running means the job is still executing
	Running JobStatus = iota
	// Failed means the job returned an error and did not complete successfully
	Failed JobStatus = iota
	// Finished means the job completed successfully
	Finished JobStatus = iota
	// Stopped means the job was stopped/killed before it could complete
	Stopped JobStatus = iota
)

// NewErrInvalidID builds a custom ErrInvalidID and returns it
func NewErrInvalidID(id string, err error) error {
	return ErrInvalidID{id, err}
}

// NewErrNoJobForID builds a custom ErrNoJobForID and returns it
func NewErrNoJobForID(id string) error {
	return ErrNoJobForID{id}
}

// ErrInvalidID is a custom error for when JobStatus(), StopJob(), or
// GetJobOutput() is called with an ID that is not a valid xid
type ErrInvalidID struct {
	id  string
	err error
}

func (inv ErrInvalidID) Error() string {
	return fmt.Sprintf("'%v' is not a valid job id: %v", inv.id, inv.err)
}

// ErrNoJobForID is a custom error for when JobStatus(), StopJob(), or
// GetJobOutput() is called with an ID that the manager doesn't know
// about
type ErrNoJobForID struct {
	id string
}

func (no ErrNoJobForID) Error() string {
	return fmt.Sprintf("no job found for id '%v'", no.id)
}
