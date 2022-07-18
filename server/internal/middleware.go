package internal

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc"
)

// Authorizer ...
type Authorizer interface {
	UserAllowed(string) bool
}

// GetUnaryAuthMiddleware ...
func GetUnaryAuthMiddleware(auth Authorizer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (resp any, err error) {
		return h(ctx, req)
	}
}

// GetStreamAuthMiddleware ...
func GetStreamAuthMiddleware(auth Authorizer) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) (err error) {
		return h(srv, stream)
	}
}

func UnaryPanicMiddleware(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during call: %v", r)
			log.Printf("caught panic! error: %v", err)
		}
	}()

	return h(ctx, req)
}

func StreamPanicMiddleware(srv interface{}, stream grpc.ServerStream,
	info *grpc.StreamServerInfo, h grpc.StreamHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during stream: %v", r)
			log.Printf("caught panic in stream! error: %v", err)
		}
	}()

	return h(srv, stream)
}
