package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func setupLogging() {
	grpc_zap.ReplaceGrpcLoggerV2(zap.L())
}

func setupUnaryMiddleware(conf Config) (grpc.ServerOption, error) {
	intercepts := []grpc.UnaryServerInterceptor{
		UnaryPanicMiddleware,
		//GetUnaryAuthMiddleware(),
	}

	intercepts = append(intercepts, conf.Interceptors.Unary...)
	return grpc_middleware.WithUnaryServerChain(intercepts...), nil
}

func setupStreamMiddleware(conf Config) (grpc.ServerOption, error) {
	intercepts := []grpc.StreamServerInterceptor{
		StreamPanicMiddleware,
	}

	intercepts = append(intercepts, conf.Interceptors.Stream...)
	return grpc_middleware.WithStreamServerChain(intercepts...), nil
}

func setupCerts(conf Config) (grpc.ServerOption, error) {
	cert, err := tls.LoadX509KeyPair(conf.CertPath, conf.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to load key pair: %w", err)
	}

	chainReader, err := os.OpenFile(conf.ChainPath, os.O_RDONLY, 0444)
	if err != nil {
		return nil, fmt.Errorf("unable to open chain file: %w", err)
	}

	bits, err := io.ReadAll(chainReader)
	if err != nil {
		return nil, fmt.Errorf("unable to read from chain file: %w", err)
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(bits); !ok {
		return nil, fmt.Errorf("unable to append cert from '%v' to cert pool", conf.ChainPath)
	}

	creds := grpc.Creds(
		credentials.NewTLS(
			&tls.Config{
				ClientAuth:   tls.RequireAndVerifyClientCert,
				Certificates: []tls.Certificate{cert},
				ClientCAs:    certPool,
			},
		),
	)

	return creds, nil
}
