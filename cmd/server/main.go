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
	keyPath := "./ca.key"
	chainPath := "./ca.pem"

	// setup config
	config := grpc.Config{
		Port: port,

		CertPath:  certPath,
		KeyPath:   keyPath,
		ChainPath: chainPath,

		// acl!
		ACL: grpc.UserPermissions{
			"admin": grpc.RPCPermissions{
				"start":  grpc.Super,
				"stop":   grpc.Super,
				"status": grpc.Super,
				"output": grpc.Super,
			},
			"alice": grpc.RPCPermissions{
				"start":  grpc.Own,
				"stop":   grpc.Own,
				"status": grpc.Own,
				"output": grpc.Own,
			},
			"bob": grpc.RPCPermissions{
				"start":  grpc.Own,
				"status": grpc.Own,
			},
			"charlie": grpc.RPCPermissions{
				"output": grpc.Super,
			},
		},
	}

	// create the server
	srv, err := grpc.NewServer(config)
	if err != nil {
		fmt.Printf("Unable to set up GRPC server: %v\n", err)
		os.Exit(1)
	}

	// create the actual service that properly handles requests
	service, err := server.NewService()
	if err != nil {
		fmt.Printf("Unable to create workernator service: %v\n", err)
		os.Exit(1)
	}

	// register our workernator /service/ with our grpc /server/
	// ...names are hare, okay?
	srv.RegisterServerHandler(func(s *grpc.GRPCServer) {
		pb.RegisterServiceServer(s, service)
	})

	// start!
	if err := srv.Start(context.Background()); err != nil {
		fmt.Printf("Unable to start GRPC server: %v\n", err)
		os.Exit(1)
	}

	// all done, nothing else to do here
	fmt.Printf("Server shutdown complete!\n")
}
