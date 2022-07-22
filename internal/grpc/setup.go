package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
		[]UnaryServerInterceptor{
			grpc_ctxtags.UnaryServerInterceptor(ctxtagOpts...),
			grpc_zap.UnaryServerInterceptor(log, grpcOpts...),
		},
		conf.Interceptors.Unary...,
	)

	conf.Interceptors.Stream = append(
		[]StreamServerInterceptor{
			grpc_ctxtags.StreamServerInterceptor(ctxtagOpts...),
			grpc_zap.StreamServerInterceptor(log, grpcOpts...),
		},
		conf.Interceptors.Stream...,
	)

	grpc_zap.ReplaceGrpcLoggerV2(log)

	return nil
}

func setupUnaryMiddleware(conf Config, auth Authorizer) (ServerOption, error) {
	intercepts := []UnaryServerInterceptor{
		UnaryPanicMiddleware,
		GetUnaryAuthMiddleware(auth),
	}

	intercepts = append(intercepts, conf.Interceptors.Unary...)
	return grpc_middleware.WithUnaryServerChain(intercepts...), nil
}

func setupStreamMiddleware(conf Config, auth Authorizer) (ServerOption, error) {
	intercepts := []StreamServerInterceptor{
		StreamPanicMiddleware,
		GetStreamAuthMiddleware(auth),
	}

	intercepts = append(intercepts, conf.Interceptors.Stream...)
	return grpc_middleware.WithStreamServerChain(intercepts...), nil
}

func setupCerts(conf Config) (ServerOption, error) {
	cert, err := conf.validateCertificate()
	if err != nil {
		return nil, err
	}

	certPool, err := conf.validateChain()
	if err != nil {
		return nil, err
	}

	creds := Creds(
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

type simpleAuth struct {
	acl UserPermissions
}

// UnaryAllowed  ...
func (sa simpleAuth) UnaryAllowed(ctx context.Context, info *UnaryServerInfo, credInfo credentials.TLSInfo) (context.Context, bool) {
	method := getRPCNameFromInfo(info.FullMethod)
	user := getUserFromCreds(credInfo)

	return sa.isUserAllowed(ctx, method, user)
}

// StreamAllowed  ...
func (sa simpleAuth) StreamAllowed(ctx context.Context, info *StreamServerInfo, credInfo credentials.TLSInfo) (context.Context, bool) {
	method := getRPCNameFromInfo(info.FullMethod)
	user := getUserFromCreds(credInfo)

	return sa.isUserAllowed(ctx, method, user)
}

func (sa simpleAuth) isUserAllowed(ctx context.Context, method string, user userInfo) (context.Context, bool) {
	if user.cn != "client" {
		return ctx, false
	}

	for i, org := range user.org {
		unit := user.orgUnit[i]
		loc := user.locality[i]

		// the 'workernator' bit should be configurable, but that can happen later
		if org != "Teleport" &&
			unit != "workernator" {
			continue
		}

		userPerms, ok := sa.acl[loc]
		if !ok {
			zap.L().Info("user not found in ACL", zap.String("username", loc))
			return ctx, false
		}

		rpcPerm, ok := userPerms[method]
		if !ok {
			zap.L().Info(
				"user does not have permission to use rpc method",
				zap.String("username", loc),
				zap.String("method", method),
			)
			return ctx, false
		}

		if rpcPerm != None {
			return putPermissionIntoContext(ctx, loc, rpcPerm), true
		}
	}

	return ctx, false
}

func getRPCNameFromInfo(fullMethod string) string {
	bits := strings.Split(fullMethod, "/")
	if len(bits) == 0 {
		return ""
	}

	return strings.ToLower(bits[len(bits)-1])
}

func getUserFromCreds(credInfo credentials.TLSInfo) userInfo {
	vc := credInfo.State.VerifiedChains
	if len(vc) == 0 {
		panic(fmt.Errorf("this shouldn't happen, no data in verififed chains"))
	}

	if len(vc[0]) == 0 {
		panic(fmt.Errorf("this shouldn't happen, no data in verififed chains"))
	}

	subj := vc[0][0].Subject

	return userInfo{
		org:      subj.Organization,
		orgUnit:  subj.OrganizationalUnit,
		locality: subj.Locality,
		cn:       subj.CommonName,
	}
}

type userInfo struct {
	org      []string
	orgUnit  []string
	locality []string
	cn       string
}

const (
	metadataUsername string = "username"
	metadataPerm     string = "permission"

	permNone  string = "none"
	permOwn   string = "own"
	permSuper string = "super"
)

var permissionMap = map[string]Permission{
	permNone:  None,
	permOwn:   Own,
	permSuper: Super,
}

func putPermissionIntoContext(ctx context.Context, username string, perm Permission) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}

	md.Set(metadataUsername, username)
	userPerm := permOwn
	if perm == Super {
		userPerm = permSuper
	}
	md.Set(metadataPerm, userPerm)

	return metadata.NewIncomingContext(ctx, md)
}

// GetUserAndPermission ...
func GetUserAndPermission(ctx context.Context) (string, Permission, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", 0, fmt.Errorf("unable to get metadata from context")
	}

	user := md.Get(metadataUsername)
	permList := md.Get(metadataPerm)

	if len(user) == 0 || len(permList) == 0 {
		return "", None, fmt.Errorf("missing user or permission in metadata")
	}

	perm, ok := permissionMap[permList[0]]
	if !ok {
		return "", None, fmt.Errorf("'%v' isn't a valid permission", permList[0])
	}

	if perm == None {
		return user[0], perm, status.Error(codes.Unauthenticated, "invalid permission")
	}

	return user[0], perm, nil
}
