package library

import "fmt"

// NewErrInvalidID ...
func NewErrInvalidID(id string, err error) ErrInvalidID {
	return ErrInvalidID{id, err}
}

// NewErrNoJobForID ...
func NewErrNoJobForID(id string) ErrNoJobForID {
	return ErrNoJobForID{id}
}

// ErrInvalidID ...
type ErrInvalidID struct {
	id  string
	err error
}

// Error ...
func (inv ErrInvalidID) Error() string {
	return fmt.Sprintf("'%v' is not a valid job id: %v", inv.id, inv.err)
}

// ErrNoJobForID ...
type ErrNoJobForID struct {
	id string
}

// Error  ...
func (no ErrNoJobForID) Error() string {
	return fmt.Sprintf("no job found for id '%v'", no.id)
}

// Job ...
type Job interface {
	JobInfo
	Wait() error
	Stop() error
}

// JobInfo ...
type JobInfo interface {
	ID() string
	Status() string
	Command() string
	Arguments() []string
	Error() error
}
