package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
)

// customLevels assigns the correct log level to the code returned by an rpc method. Most
// codes are assigned to the 'info' level.
//
// Available levels, and when they should be used ( here or elsewhere )
//  - debug  --> unused
//  - info   --> default
//  - warn   --> use for codes related to user error
//  - error  --> use for codes related to a server-side error
//  - dpanic --> only use for critical rpc methods
//  - panic  --> never use
//  - fatal  --> never use
func customLevels(code codes.Code) zapcore.Level {
	var out zapcore.Level
	switch code {
	default:
		// by default, everything goes to debug
		out = zapcore.InfoLevel

	case codes.OutOfRange:
		fallthrough
	case codes.FailedPrecondition:
		fallthrough
	case codes.Aborted:
		fallthrough
	case codes.PermissionDenied:
		fallthrough
	case codes.InvalidArgument:
		fallthrough
	case codes.Unauthenticated:
		fallthrough
	case codes.Canceled:
		// something caused on user end?
		out = zapcore.WarnLevel

	case codes.Internal:
		fallthrough
	case codes.DataLoss:
		fallthrough
	case codes.ResourceExhausted:
		fallthrough
	case codes.AlreadyExists:
		fallthrough
	case codes.NotFound:
		// resource not found on server end
		out = zapcore.ErrorLevel
	}

	return out
}

var _ grpc_zap.CodeToLevel = customLevels

func setupLogging(conf *Config) error {
	grpcOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(customLevels),
	}

	encConf := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		TimeKey:        "time",
		NameKey:        "logger",
		CallerKey:      "file",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}

	initialFields := map[string]interface{}{
		"app": "workernator",
	}

	infoLevel := zap.NewAtomicLevel()
	infoLevel.SetLevel(zap.InfoLevel)

	zc := zap.Config{
		Level:             infoLevel,
		Development:       conf.DevMode,
		DisableCaller:     false,
		DisableStacktrace: false,
		Encoding:          "console",
		EncoderConfig:     encConf,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
		InitialFields:     initialFields,
	}

	opts := []zap.Option{
		zap.AddCallerSkip(1),
	}

	log, err := zc.Build(opts...)
	if err != nil {
		return fmt.Errorf("unable to set up logging: %w", err)
	}
	zap.ReplaceGlobals(log)

	ctxtagOpts := []grpc_ctxtags.Option{
		grpc_ctxtags.WithFieldExtractor(
			grpc_ctxtags.CodeGenRequestFieldExtractor,
		),
	}

	conf.Interceptors.Unary = append(
		conf.Interceptors.Unary,
		grpc_ctxtags.UnaryServerInterceptor(ctxtagOpts...),
		grpc_zap.UnaryServerInterceptor(log, grpcOpts...),
	)

	conf.Interceptors.Stream = append(
		conf.Interceptors.Stream,
		grpc_ctxtags.StreamServerInterceptor(ctxtagOpts...),
		grpc_zap.StreamServerInterceptor(log, grpcOpts...),
	)

	grpc_zap.ReplaceGrpcLoggerV2(log)

	return nil
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
				MinVersion:   tls.VersionTLS13,
			},
		),
	)

	return creds, nil
}
