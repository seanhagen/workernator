package server

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/rs/xid"
	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Manager is the interface expected by the service that it'll use to
// manage jobs on behalf of callers.
type Manager interface {
	StartJob(context.Context, string, ...string) (*library.Job, error)
	StopJob(context.Context, string) (*library.JobInfo, error)
	JobStatus(context.Context, string) (*library.JobInfo, error)
	GetJobOutput(context.Context, string) (io.Reader, error)
}

// Service is the implementation of the Workernator GRPC service
type Service struct {
	pb.UnimplementedServiceServer

	manager Manager
}

// NewService builds a Service, returning an error if there are any issues encountered
// while setting up the service.
func NewService(mgr Manager) (*Service, error) {
	return &Service{manager: mgr}, nil
}

func jobinfoToProtobuf(in *library.JobInfo) *pb.Job {
	out := &pb.Job{
		Id:        in.ID,
		Status:    pb.JobStatus(in.Status),
		Command:   in.Command,
		Args:      in.Arguments,
		StartedAt: timestamppb.New(in.Started),
	}

	if in.Error != nil {
		out.ErrorMsg = in.Error.Error()
	}

	if !in.Ended.IsZero() {
		out.EndedAt = timestamppb.New(in.Ended)
	}

	return out
}

// Start handles starting a job
func (s *Service) Start(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
	job, err := s.manager.StartJob(ctx, req.GetCommand(), req.GetArguments()...)
	if err != nil {
		return nil, err
	}
	info := job.Info()
	return jobinfoToProtobuf(&info), nil
}

// Stop handles stopping a job
func (s *Service) Stop(ctx context.Context, req *pb.JobStopRequest) (*pb.Job, error) {
	tmp := strings.TrimSpace(req.GetId())
	id, err := xid.FromString(tmp)
	if err != nil {
		// also set the response code to codes.InvalidArguments
		return nil, err
	}

	job, err := s.manager.StopJob(ctx, id.String())
	if err != nil {
		return nil, err
	}

	return jobinfoToProtobuf(job), nil
}

// Status handles returning the status of any running or finished jobs
func (s *Service) Status(ctx context.Context, req *pb.JobStatusRequest) (*pb.Job, error) {
	tmp := strings.TrimSpace(req.GetId())
	id, err := xid.FromString(tmp)
	if err != nil {
		// also set the response code to codes.InvalidArguments
		return nil, err
	}

	job, err := s.manager.JobStatus(ctx, id.String())
	if err != nil {
		return nil, err
	}

	return jobinfoToProtobuf(job), nil
}

// Output handles streaming the output of any running or finished jobs
func (s *Service) Output(req *pb.OutputJobRequest, strm pb.Service_OutputServer) error {
	tmp := strings.TrimSpace(req.GetId())
	id, err := xid.FromString(tmp)
	if err != nil {
		// also set the response code to codes.InvalidArguments
		return err
	}

	output, err := s.manager.GetJobOutput(strm.Context(), id.String())
	if err != nil {
		// also set the response code to codes.Internal
		return err
	}

	var buf = make([]byte, 1024)

	for {
		n, err := output.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("unable to read from buffer: %w", err)
		}

		toSend := &pb.OutputJobResponse{Data: buf[:n]}
		if err := strm.Send(toSend); err != nil {
			return fmt.Errorf("unable to send to stream: %w", err)
		}
	}

	return nil
}
