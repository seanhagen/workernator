package main

import (
	"context"
	"fmt"
	"os"

	"github.com/seanhagen/workernator/internal/grpc"
	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library/server"
)

func main() {
	// parse flags
	port := "8080"
	certPath := "./server.pem"
	keyPath := "./cakey.key"
	chainPath := "./ca.pem"

	// setup config
	config := grpc.Config{
		Port: port,

		CertPath:  certPath,
		KeyPath:   keyPath,
		ChainPath: chainPath,

		ACL: grpc.UserPermissions{
			"admin": grpc.RPCPermissions{},
		},
	}

	// create the server
	srv, err := grpc.NewServer(config)
	if err != nil {
		fmt.Printf("Unable to set up GRPC server: %v\n", err)
		os.Exit(1)
	}

	service, err := server.NewService()
	if err != nil {
		fmt.Printf("Unable to create workernator service: %v\n", err)
		os.Exit(1)
	}

	srv.RegisterServerHandler(func(s *grpc.GRPCServer) {
		pb.RegisterServiceServer(s, service)
	})

	// get it started!
	fmt.Printf("Starting GRPC server!\n")
	if err := srv.Start(context.Background()); err != nil {
		fmt.Printf("Unable to start GRPC server: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Server shutdown complete!\n")
}
