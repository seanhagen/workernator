package grpc

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Authorizer is an interface used by both the unary and stream middleware
// for checking that users are authorized to use a specific route.
type Authorizer interface {
	UnaryAllowed(ctx context.Context, info *UnaryServerInfo, credInfo credentials.TLSInfo) (context.Context, bool)
	StreamAllowed(ctx context.Context, info *StreamServerInfo, credInfo credentials.TLSInfo) (context.Context, bool)
}

// GetUnaryAuthMiddleware uses an Authorizer to validate that users have the correct
// permissions for the route they're attempting to access.
func GetUnaryAuthMiddleware(auth Authorizer) UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *UnaryServerInfo, h UnaryHandler) (resp any, err error) {
		pr, ok := peer.FromContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "unable to authorize")
		}

		tlsinfo, ok := pr.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return nil, status.Error(codes.Internal, "unable to get tls credential info from context")
		}

		if ctx, ok := auth.UnaryAllowed(ctx, info, tlsinfo); ok {
			return h(ctx, req)
		}

		return nil, status.Error(codes.Unauthenticated, "unable to authorize")
	}
}

// GetStreamAuthMiddleware uses an Authorizer to validate that users have the correct
// permissions for the route they're attempting to access.
func GetStreamAuthMiddleware(auth Authorizer) StreamServerInterceptor {
	return func(srv any, stream GRPCServerStream, info *StreamServerInfo, h StreamHandler) (err error) {
		ctx := stream.Context()
		pr, ok := peer.FromContext(ctx)
		if !ok {
			return status.Error(codes.Unauthenticated, "unable to authorize")
		}

		tlsinfo, ok := pr.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return status.Error(codes.Internal, "unable to get tls credential info from context")
		}

		if ctx, ok := auth.StreamAllowed(stream.Context(), info, tlsinfo); ok {
			stream = ServerStream{stream, ctx}
			return h(srv, stream)
		}

		return status.Error(codes.Unauthenticated, "unable to authenticate")
	}
}

// UnaryPanicMiddleware catches any panics that happen during runtime
// and prints them as error logs, preventing the server from exiting
// due to a panic.
func UnaryPanicMiddleware(ctx context.Context, req interface{},
	info *UnaryServerInfo, h UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during call: %v", r)
			zap.L().Error("caught panic", zap.Error(err))
		}
	}()

	return h(ctx, req)
}

// StreamPanicMiddleware catches any panics that happen during runtime
// and prints them as error logs, preventing the server from exiting
// due to a panic.
func StreamPanicMiddleware(srv interface{}, stream GRPCServerStream,
	info *StreamServerInfo, h StreamHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during stream: %v", r)
			zap.L().Error("caught panic", zap.Error(err))
		}
	}()

	return h(srv, stream)
}
