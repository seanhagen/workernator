package server

import (
	"context"

	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library"
)

// Manager ...
type Manager interface {
	StartJob(ctx context.Context, cmd string, args ...string) (library.Job, error)
	Status(ctx context.Context, id string)
}

// Service is
type Service struct {
	pb.UnimplementedServiceServer
}

// NewService ...
func NewService() (*Service, error) {
	return &Service{}, nil
}

// // Start ...
// func (s *Service) Start(context.Context, *pb.JobStartRequest) (*pb.Job, error) {
// 	return nil, fmt.Errorf("not yet")
// }

// // Stop ...
// func (s *Service) Stop(context.Context, *pb.JobStopRequest) (*pb.Job, error) {
// 	return nil, fmt.Errorf("not yet")
// }

// // Status ...
// func (s *Service) Status(context.Context, *pb.JobStatusRequest) (*pb.Job, error) {
// 	return nil, fmt.Errorf("not yet")
// }

// // Output ...
// func (s *Service) Output(*pb.OutputJobRequest, pb.Service_OutputServer) error {
// 	return fmt.Errorf("not yet")
// }
