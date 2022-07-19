package client

import (
	"context"

	pb "github.com/seanhagen/workernator/internal/pb"
)

// TestStart uses the setup client to attempt to start a job. The GRPC
// client runs against a local test server started by the test; the
// method used by this test server to handle a start job request is
// the `cts.server.startHandle` function that's created. Once the
// server is started with `cts.startServer()` the client can be called
// to test the functionality.
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
