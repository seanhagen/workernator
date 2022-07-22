package main

import (
	"context"

	"github.com/seanhagen/workernator/internal/grpc"
	"github.com/seanhagen/workernator/internal/pb"
	"github.com/seanhagen/workernator/library/api"
	"github.com/seanhagen/workernator/library/server"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func main() {
	cobra.CheckErr(rootCmd.Execute())
}

var rootCmd = &cobra.Command{
	Use:   api.WorkernatorServerCmdName,
	Short: "The GRPC server for workernator",
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(api.RunInNamespaceCmd())
	rootCmd.AddCommand(api.LaunchJobCmd())

	serveCmd.Flags().StringVarP(&port, "port", "p", "", "what port the server should listen on")
	serveCmd.Flags().StringVarP(
		&certPath, "certs", "c", "",
		"path to a valid TLS certificate that the server will use to identify itself to clients")
	serveCmd.Flags().StringVarP(&keyPath, "key", "k", "", "path to a valid key file")
	serveCmd.Flags().StringVarP(&chainPath, "chain", "a", "", "path to a valid CA certificate")
	serveCmd.Flags().StringVarP(&outputPath, "output", "o", "", "path to a folder the server can write to")
}

var (
	port       string
	certPath   string
	keyPath    string
	chainPath  string
	outputPath string
)

var defaultACL = grpc.UserPermissions{
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
}

var serverConfig grpc.Config
var managerConfig api.Config

var serveCmd = &cobra.Command{
	Use:   "run",
	Short: "Starts the GRPC server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// setup config
		serverConfig = grpc.Config{
			Port: port,

			CertPath:  certPath,
			KeyPath:   keyPath,
			ChainPath: chainPath,

			// acl!
			ACL: defaultACL,
		}
		if err := serverConfig.Valid(); err != nil {
			return err
		}

		managerConfig = api.Config{
			OutputPath: outputPath,
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := api.NewManager(managerConfig)
		if err != nil {
			zap.L().Fatal("Unable to set up job manager", zap.Error(err))
		}

		// create the server
		srv, err := grpc.NewServer(serverConfig)
		if err != nil {
			zap.L().Fatal("Unable to set up GRPC server", zap.Error(err))
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

		return nil
	},
}
