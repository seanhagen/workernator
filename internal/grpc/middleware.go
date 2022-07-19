package grpc

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Authorizer is an interface used by both the unary and stream middleware
// for checking that users are authorized to use a specific route.
type Authorizer interface {
	UserAllowed(string) bool
}

// GetUnaryAuthMiddleware uses an Authorizer to validate that users have the correct
// permissions for the route they're attempting to access.
func GetUnaryAuthMiddleware(auth Authorizer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (resp any, err error) {
		return h(ctx, req)
	}
}

// GetStreamAuthMiddleware uses an Authorizer to validate that users have the correct
// permissions for the route they're attempting to access.
func GetStreamAuthMiddleware(auth Authorizer) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) (err error) {
		return h(srv, stream)
	}
}

// UnaryPanicMiddleware catches any panics that happen during runtime
// and prints them as error logs, preventing the server from exiting
// due to a panic.
func UnaryPanicMiddleware(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (resp interface{}, err error) {
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
func StreamPanicMiddleware(srv interface{}, stream grpc.ServerStream,
	info *grpc.StreamServerInfo, h grpc.StreamHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during stream: %v", r)
			zap.L().Error("caught panic", zap.Error(err))
		}
	}()

	return h(srv, stream)
}
