package worker

import (
	"context"

	pb "github.com/seanhagen/workernator/internal/pb"
)

// Service is
type Service struct {
	pb.UnimplementedServiceServer
}

// NewService ...
func NewService() (*Service, error) {
	return &Service{}, nil
}

// Start ...
func (s *Service) Start(context.Context, *pb.JobStartRequest) (*pb.Job, error) {

}

// Stop ...
func (s *Service) Stop(context.Context, *pb.JobStopRequest) (*pb.Job, error) {}

// Status ...
func (s *Service) Status(context.Context, *pb.JobStatusRequest) (*pb.Job, error) {}

// Output ...
func (s *Service) Output(*pb.OutputJobRequest, pb.Service_OutputServer) error {}
