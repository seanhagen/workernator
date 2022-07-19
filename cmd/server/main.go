package main

import (
	"context"
	"fmt"
	"os"

	"github.com/seanhagen/workernator/internal/grpc"
	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library/api"
	"github.com/seanhagen/workernator/library/server"
	"go.uber.org/zap"
)

func main() {
	// parse flags
	port := "8080"
	certPath := "./server.pem"
	keyPath := "./ca.key"
	chainPath := "./ca.pem"

	outputPath, err := os.MkdirTemp("/tmp", "workernator")
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create temporary directory for job output: %v", err)
		os.Exit(1)
	}

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
		zap.L().Fatal("Unable to set up GRPC server", zap.Error(err))
	}

	managerConfig := api.Config{
		OutputPath: outputPath,
	}

	manager, err := api.NewManager(managerConfig)
	if err != nil {
		zap.L().Fatal("Unable to set up job manager", zap.Error(err))
	}

	// create the actual service that properly handles requests
	service, err := server.NewService(manager)
	if err != nil {
		zap.L().Fatal("Unable to create workernator service", zap.Error(err))
	}

	// register our workernator /service/ with our grpc /server/; names are hard, okay?
	srv.RegisterServerHandler(func(s *grpc.GRPCServer) {
		pb.RegisterServiceServer(s, service)
	})

	// start!
	if err := srv.Start(context.Background()); err != nil {
		zap.L().Fatal("Unable to start GRPC server", zap.Error(err))
	}

	zap.L().Info("Server shutdown complete!")
}
