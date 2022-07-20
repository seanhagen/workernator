package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/rs/xid"
	"github.com/seanhagen/workernator/internal/grpc"
	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Manager is the interface expected by the service that it'll use to
// manage jobs on behalf of callers.
type Manager interface {
	StartJob(context.Context, string, ...string) (*library.Job, error)
	StopJob(context.Context, string) (*library.JobInfo, error)
	JobStatus(context.Context, string) (*library.JobInfo, error)
	GetJobOutput(context.Context, string) (io.ReadCloser, error)
}

// Service is the implementation of the Workernator GRPC service
type Service struct {
	pb.UnimplementedServiceServer

	manager Manager

	jobsByUser sync.Map
	jkLock     sync.RWMutex
	jobsKnown  []string
}

// NewService builds a Service, returning an error if there are any issues encountered
// while setting up the service.
func NewService(mgr Manager) (*Service, error) {
	svc := &Service{
		manager: mgr,

		jobsByUser: sync.Map{},
		jkLock:     sync.RWMutex{},
		jobsKnown:  []string{},
	}

	return svc, nil
}

// storeUserJob  ...
func (s *Service) storeUserJob(user, job string) {
	s.jobsByUser.Store(user, job)
	s.jkLock.Lock()
	s.jobsKnown = append(s.jobsKnown, job)
	s.jkLock.Unlock()
}

// userCreatedJob ...
func (s *Service) userCreatedJob(perm grpc.Permission, user, job string) bool {
	if perm == grpc.Super {
		return true
	}

	tmp, ok := s.jobsByUser.Load(user)
	if !ok {
		return false
	}

	str, ok := tmp.(string)
	if !ok {
		return false
	}

	return str == job
}

// isKnownJob ...
func (s *Service) isKnownJob(job string) bool {
	s.jkLock.RLock()
	defer s.jkLock.RUnlock()

	for _, jid := range s.jobsKnown {
		if jid == job {
			return true
		}
	}

	return false
}

// Start handles starting a job
func (s *Service) Start(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
	user, _, err := grpc.GetUserAndPermission(ctx)
	if err != nil {
		// GetUserAndPermission returns an error if the permission is
		// None, so we don't need to check for that separately
		return nil, err
	}

	job, err := s.manager.StartJob(ctx, req.GetCommand(), req.GetArguments()...)
	if err != nil {
		return nil, err
	}

	s.storeUserJob(user, job.ID)

	info := job.Info()
	return jobinfoToProtobuf(&info), nil
}

// Stop handles stopping a job
func (s *Service) Stop(ctx context.Context, req *pb.JobStopRequest) (*pb.Job, error) {
	user, perm, err := grpc.GetUserAndPermission(ctx)
	if err != nil {
		return nil, err
	}

	tmp := strings.TrimSpace(req.GetId())
	id, err := xid.FromString(tmp)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("'%v' is not a valid id: %v", tmp, err))
	}

	// if we don't know what the job is or the user doesn't have the
	// permissions to stop the job then we return a 'not found' error
	if !s.isKnownJob(id.String()) || !s.userCreatedJob(perm, user, id.String()) {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("no job found for id '%s'", id.String()))
	}

	job, err := s.manager.StopJob(ctx, id.String())
	if err != nil {
		return nil, err
	}

	return jobinfoToProtobuf(job), nil
}

// Status handles returning the status of any running or finished jobs
func (s *Service) Status(ctx context.Context, req *pb.JobStatusRequest) (*pb.Job, error) {
	user, perm, err := grpc.GetUserAndPermission(ctx)
	if err != nil {
		return nil, err
	}

	tmp := strings.TrimSpace(req.GetId())
	id, err := xid.FromString(tmp)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("'%v' is not a valid id: %v", tmp, err))
	}

	if !s.isKnownJob(id.String()) || !s.userCreatedJob(perm, user, id.String()) {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("no job found for id '%s'", id.String()))
	}

	job, err := s.manager.JobStatus(ctx, id.String())
	if err != nil {
		return nil, err
	}

	return jobinfoToProtobuf(job), nil
}

// Output handles streaming the output of any running or finished jobs
func (s *Service) Output(req *pb.OutputJobRequest, strm pb.Service_OutputServer) error {
	user, perm, err := grpc.GetUserAndPermission(strm.Context())
	if err != nil {
		return err
	}

	tmp := strings.TrimSpace(req.GetId())
	id, err := xid.FromString(tmp)
	if err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("'%v' is not a valid id: %v", tmp, err))
	}

	if !s.isKnownJob(id.String()) || !s.userCreatedJob(perm, user, id.String()) {
		return status.Error(codes.NotFound, fmt.Sprintf("no job found for id '%s'", id.String()))
	}

	output, err := s.manager.GetJobOutput(strm.Context(), id.String())
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("unable to get job output: %v", err))
	}

	go func() {
		<-strm.Context().Done()
		if err := output.Close(); err != nil {
			zap.L().Error("unable to close job output reader", zap.Error(err))
		}
	}()

	var buf = make([]byte, 1024)

	for {
		n, err := output.Read(buf)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return status.Error(codes.Internal, fmt.Sprintf("unable to read from buffer: %v", err))
		}

		toSend := &pb.OutputJobResponse{Data: buf[:n]}
		if err := strm.Send(toSend); err != nil {
			return status.Error(codes.Internal, fmt.Sprintf("unable to send to stream: %s", err))
		}
	}

	return nil
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
