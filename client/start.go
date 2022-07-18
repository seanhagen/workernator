package client

import (
	"context"
	"fmt"

	pb "github.com/seanhagen/workernator/internal/pb"
)

// StartJob ...
func (c *Client) StartJob(ctx context.Context, command string, arguments ...string) (Job, error) {
	req := pb.JobStartRequest{
		Command:   command,
		Arguments: arguments,
	}

	resp, err := c.grpc.Start(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("unable to start job: %w", err)
	}

	return startResp{id: resp.Id}, nil
}

type startResp struct {
	id string
}

// ID ...
func (sr startResp) ID() string {
	return sr.id
}
