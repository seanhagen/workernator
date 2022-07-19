package server

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"time"

	"github.com/rs/xid"
	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed king_lear.html
var kingLear string

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

// Start ...
func (s *Service) Start(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
	return debugOutput(), nil
}

// Stop ...
func (s *Service) Stop(ctx context.Context, req *pb.JobStopRequest) (*pb.Job, error) {
	return debugOutput(), nil
}

// Status ...
func (s *Service) Status(ctx context.Context, req *pb.JobStatusRequest) (*pb.Job, error) {
	return debugOutput(), nil
}

// Output ...
func (s *Service) Output(req *pb.OutputJobRequest, strm pb.Service_OutputServer) error {
	err := strm.Send(&pb.OutputJobResponse{
		Data: []byte("thanks for asking for the output of job '" + req.GetId() + "'\n"),
	})
	if err != nil {
		return fmt.Errorf("unable to send to client: %w", err)
	}

	strm.Send(&pb.OutputJobResponse{
		Data: []byte("this method doesn't do anything yet, so have all of king lear as an html file.\n\n\n"),
	})
	if err != nil {
		return fmt.Errorf("unable to send to client: %w", err)
	}

	// this buffer will be swapped out with the io.Reader we get from the library
	// when it's reading the output of a job
	kingLearBuffer := bytes.NewBufferString(kingLear)

	var buf []byte = make([]byte, 1024)

	for {
		n, err := kingLearBuffer.Read(buf)
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

func debugOutput() *pb.Job {
	id := xid.New()
	ended := time.Now()
	started := ended.Add(time.Minute * -1)

	return &pb.Job{
		Id:       id.String(),
		Status:   pb.JobStatus_Finished,
		Command:  "echo",
		Args:     []string{"hello world"},
		ErrorMsg: "",

		StartedAt: timestamppb.New(started),
		EndedAt:   timestamppb.New(ended),
	}
}
