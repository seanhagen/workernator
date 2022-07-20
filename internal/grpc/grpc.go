package grpc

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// GRPCServer is a type alias for grpc.Server
type GRPCServer = grpc.Server

type UnaryServerInterceptor = grpc.UnaryServerInterceptor
type StreamServerInterceptor = grpc.StreamServerInterceptor
type ServerOption = grpc.ServerOption
type DialOption = grpc.DialOption
type UnaryServerInfo = grpc.UnaryServerInfo
type UnaryHandler = grpc.UnaryHandler
type StreamServerInfo = grpc.StreamServerInfo
type StreamHandler = grpc.StreamHandler
type GRPCServerStream = grpc.ServerStream

var WithTransportCredentials = grpc.WithTransportCredentials
var Creds = grpc.Creds
var DialContext = grpc.DialContext
var WithContextDialer = grpc.WithContextDialer

type ServerStream struct {
	GRPCServerStream

	ctx context.Context
}

// Context ...
func (ss ServerStream) Context() context.Context {
	return ss.ctx
}

// GRPCHandler is a function type accepted by RegisterServerHandler so
// users of the Server object can register their services
type GRPCHandler func(*grpc.Server)

// Server wraps up a *grpc.Server and provides a really simple wrapper
type Server struct {
	srv    *grpc.Server
	listen net.Listener
	config Config
}

// NewServer uses the provided Config to build a GRPC server that can
// be used to handle client requests.
func NewServer(conf Config) (*Server, error) {
	if err := conf.Valid(); err != nil {
		return nil, fmt.Errorf("can't configure server, invalid configuration: %w", err)
	}

	l, err := net.Listen("tcp", ":"+conf.Port)
	if err != nil {
		return nil, fmt.Errorf("can't listen on port %v, encountered error: %w", conf.Port, err)
	}

	auth := simpleAuth{conf.ACL}

	server := &Server{
		listen: l,
		config: conf,
	}

	if err := setupLogging(&conf); err != nil {
		return nil, fmt.Errorf("unable to setup logging: %w", err)
	}

	mtlsConfig, err := setupCerts(conf)
	if err != nil {
		return nil, fmt.Errorf("unable to setup mTLS configuration: %w", err)
	}

	unaryInterceptors, err := setupUnaryMiddleware(conf, auth)
	if err != nil {
		return nil, fmt.Errorf("unable to setup unary interceptors: %w", err)
	}

	streamInterceptors, err := setupStreamMiddleware(conf, auth)
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

// RegisterServerHandler is used to register services with the
// *grpc.Server we've got wrapped up.
func (s *Server) RegisterServerHandler(hn GRPCHandler) {
	hn(s.srv)
}
