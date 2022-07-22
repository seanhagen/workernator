package client

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/rs/xid"
	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library"
	"go.uber.org/zap"
)

// StartJob reaches out to the server to ask it to run a command for
// us as a job.
func (c *Client) StartJob(ctx context.Context, command string, arguments ...string) (*library.JobInfo, error) {
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

// StopJob reaches out to the server to ask it to stop a running job
// for us. If there is an issue killing the job an error will be
// returned, otherwise the only errors should be related to network
// issues ( can't reach host, etc ).
//
// This function is idempotent, it can be called multiple times with
// the same ID ( so long as it's a valid ID ) and it will return the
// same result.
func (c *Client) StopJob(ctx context.Context, id string) (*library.JobInfo, error) {
	_, err := xid.FromString(id)
	if err != nil {
		return nil, fmt.Errorf("'%s' is not a valid ID: %w", id, err)
	}
	req := pb.JobStopRequest{Id: id}

	resp, err := c.grpc.Stop(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("unable to stop job: %w", err)
	}

	return grpcJobToClientJob(resp), nil
}

// JobStatus reaches out to the server to ask for the status of a
// job. If the ID given is either invalid or doesn't map to a job, an
// error will be returned.
func (c *Client) JobStatus(ctx context.Context, id string) (*library.JobInfo, error) {
	_, err := xid.FromString(id)
	if err != nil {
		return nil, fmt.Errorf("'%s' is not a valid ID: %w", id, err)
	}

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
	_, err := xid.FromString(id)
	if err != nil {
		return nil, fmt.Errorf("'%s' is not a valid ID: %w", id, err)
	}

	req := pb.OutputJobRequest{Id: id}
	strm, err := c.grpc.Output(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("unable to start output stream: %w", err)
	}

	read, write := io.Pipe()

	go func() {
		defer func() {
			err := strm.CloseSend()
			if err != nil {
				zap.L().Error("error closing stream", zap.Error(err))
			}
		}()

		for {
			msg, err := strm.Recv()
			if err != nil {
				_ = write.CloseWithError(err)
				return
			}

			_, err = write.Write(msg.Data)
			if err != nil {
				_ = write.CloseWithError(fmt.Errorf("unable to write data: %w", err))
				return
			}
		}
	}()

	return read, nil
}

func grpcJobToClientJob(resp *pb.Job) *library.JobInfo {
	var jobErr error
	errMsg := strings.TrimSpace(resp.GetErrorMsg())
	if errMsg != "" {
		jobErr = fmt.Errorf(errMsg)
	}

	out := library.JobInfo{
		ID:        resp.GetId(),
		Status:    library.JobStatus(resp.GetStatus().Number()),
		Command:   resp.GetCommand(),
		Arguments: resp.GetArgs(),
		Error:     jobErr,
		Started:   resp.GetStartedAt().AsTime(),
	}

	if resp.EndedAt != nil {
		out.Ended = resp.GetEndedAt().AsTime()
	}

	return &out
}
