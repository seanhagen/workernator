package client

import (
	"context"
	"fmt"
	"io"
	"strings"

	pb "github.com/seanhagen/workernator/internal/pb"
)

func (c *Client) StartJob(ctx context.Context, command string, arguments ...string) (*JobResponse, error) {
// StartJob reaches out to the server to ask it to run a command for
// us as a job.
	req := pb.JobStartRequest{
		Command:   command,
		Arguments: arguments,
	}

	resp, err := c.grpc.Start(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("unable to start job: %w", err)
	}

	return grpcJobToClientJob(resp), nil
}

func (c *Client) StopJob(ctx context.Context, id string) error {
// StopJob reaches out to the server to ask it to stop a running job
// for us. If there is an issue killing the job an error will be
// returned, otherwise the only errors should be related to network
// issues ( can't reach host, etc ).
//
// This function is idempotent, it can be called multiple times with
// the same ID ( so long as it's a valid ID ) and it will return the
// same result.
	req := pb.JobStopRequest{Id: id}

	resp, err := c.grpc.Stop(ctx, &req)
	if err != nil {
		return fmt.Errorf("unable to stop job: %w", err)
	}

	errMsg := strings.TrimSpace(resp.GetErrorMsg())
	if errMsg != "" {
		return fmt.Errorf(errMsg)
	}

	return nil
}

func (c *Client) JobStatus(ctx context.Context, id string) (*JobResponse, error) {
// JobStatus reaches out to the server to ask for the status of a
// job. If the ID given is either invalid or doesn't map to a job, an
// error will be returned.
	req := pb.JobStatusRequest{Id: id}
	resp, err := c.grpc.Status(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("unable to get job status: %w", err)
	}

	return grpcJobToClientJob(resp), nil
}

// JobOutput reaches out to the server to request the output be
// streamed to the client.  The function returns an io.Reader that
// when read will return the output from the job. If the ID given is
// either invalid or doesn't map to a job, an error will be returned.
func (c *Client) JobOutput(ctx context.Context, id string) (io.Reader, error) {
	req := pb.OutputJobRequest{Id: id}
	strm, err := c.grpc.Output(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("unable to start output stream: %v", err)
	}

	read, write := io.Pipe()

	go func() {
		defer func() {
			err := strm.CloseSend()
			if err != nil {
				fmt.Printf("\n\nerror closing stream: %v\n", err)
			}
		}()

		for {
			msg, err := strm.Recv()
			if err != nil {
				write.CloseWithError(err)

				return
			}

			_, err = write.Write(msg.Data)
			if err != nil {
				write.CloseWithError(fmt.Errorf("unable to write data: %v", err))
				return
			}
		}
	}()

	return read, nil
}

func grpcJobToClientJob(resp *pb.Job) *JobResponse {
	var jobErr error
	errMsg := strings.TrimSpace(resp.GetErrorMsg())
	if errMsg != "" {
		jobErr = fmt.Errorf(errMsg)
	}

	out := JobResponse{
		ID:      resp.GetId(),
		Status:  JobStatus(resp.GetStatus().Number()),
		Cmd:     resp.GetCommand(),
		Args:    resp.GetArgs(),
		Err:     jobErr,
		Started: resp.GetStartedAt().AsTime(),
	}

	if resp.EndedAt != nil {
		tm := resp.GetEndedAt().AsTime()
		out.Ended = tm
	}

	return &out
}
