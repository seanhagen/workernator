package client

import (
	"context"

	pb "github.com/seanhagen/workernator/internal/pb"
)

// TestStart  ...
func (cts *ClientTestSuite) TestStart() {
	ctx := context.TODO()

	expectID := "test-id"

	cts.server.startHandle = func(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
		return &pb.Job{Id: expectID}, nil
	}

	cts.startServer()

	job, err := cts.client.StartJob(ctx, "testing", "one", "two")
	cts.NoError(err)
	cts.NotNil(job)
	cts.Equal(expectID, job.ID())
}
