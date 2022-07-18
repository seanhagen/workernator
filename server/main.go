package main

import (
	"context"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/server/internal"
	"google.golang.org/grpc"
)

func main() {
	// parse flags
	port := "8080"
	certPath := "./server.pem"
	keyPath := "./cakey.key"
	chainPath := "./ca.pem"

	// setup config
	config := internal.Config{
		Port: port,

		CertPath:  certPath,
		KeyPath:   keyPath,
		ChainPath: chainPath,

		ACL: internal.UserPermissions{
			"admin": internal.RPCPermissions{},
		},
	}

	// create the server
	srv, err := internal.NewServer(config)
	if err != nil {
		fmt.Printf("Unable to set up GRPC server: %v\n", err)
		os.Exit(1)
	}

	srv.RegisterServerHandler(func(s *grpc.Server) {
		pb.RegisterServiceServer(s, &skeleton{})
	})

	// get it started!
	fmt.Printf("Starting GRPC server!\n")
	if err := srv.Start(context.Background()); err != nil {
		fmt.Printf("Unable to start GRPC server: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Server shutdown complete!\n")
}

type skeleton struct {
	pb.UnimplementedServiceServer
}

// Start  ...
func (sk *skeleton) Start(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
	spew.Dump(req)
	return &pb.Job{Id: "this-is-a-test"}, nil
}
