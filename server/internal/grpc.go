package internal

import (
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	handshakeTimeout = time.Second * 10
)

// GRPCHandler ...
type GRPCHandler func(*grpc.Server)

// Server ...
type Server struct {
	srv    *grpc.Server
	listen net.Listener
	config Config
}

// NewServer ...
func NewServer(conf Config) (*Server, error) {
	if err := conf.Valid(); err != nil {
		return nil, fmt.Errorf("can't configure server, invalid configuration: %w", err)
	}

	l, err := net.Listen("tcp", ":"+conf.Port)
	if err != nil {
		return nil, fmt.Errorf("can't listen on port %v, encountered error: %w", conf.Port, err)
	}

	server := &Server{
		listen: l,
		config: conf,
	}

	setupLogging()

	mtlsConfig, err := setupCerts(conf)
	if err != nil {
		return nil, fmt.Errorf("unable to setup mTLS configuration: %w", err)
	}

	unaryInterceptors, err := setupUnaryMiddleware(conf)
	if err != nil {
		return nil, fmt.Errorf("unable to setup unary interceptors: %w", err)
	}

	streamInterceptors, err := setupStreamMiddleware(conf)
	if err != nil {
		return nil, fmt.Errorf("unable to setup stream interceptors: %w", err)
	}

	srvOpts := []grpc.ServerOption{
		unaryInterceptors,
		streamInterceptors,
		mtlsConfig,
	}

	grpcServer := grpc.NewServer(srvOpts...)
	reflection.Register(grpcServer)

	server.srv = grpcServer

	return server, nil
}

// RegisterServerHandler  ...
func (s *Server) RegisterServerHandler(hn GRPCHandler) {
	hn(s.srv)
}
