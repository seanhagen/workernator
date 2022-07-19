package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"testing"

	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func TestInternal_NewServer(t *testing.T) {
	ctx := context.TODO()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	conf := Config{
		Port: "8080",

		CertPath:  "./testdata/server.pem",
		KeyPath:   "./testdata/cakey.key",
		ChainPath: "./testdata/ca.pem",

		ACL: UserPermissions{
			"admin": RPCPermissions{
				"start":  Super,
				"stop":   Super,
				"status": Super,
				"output": Super,
			},
		},
	}

	listener := bufconn.Listen(bufSize)

	server, err := NewServer(conf)
	require.NoError(t, err)

	server.listen = listener

	expect := &pb.Job{
		Id: "this-is-a-test",
	}

	skel := &skeleton{
		handleStart: func(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
			return expect, nil
		},
	}

	server.RegisterServerHandler(func(s *grpc.Server) {
		pb.RegisterServiceServer(s, skel)
	})

	go func() {
		err = server.Start(ctx)
		require.NoError(t, err)
	}()

	t.Cleanup(func() {
		if err := server.stop(ctx); err != nil {
			t.Errorf("error stopping server: %v", err)
		}
	})

	tlsDialOpt, err := setupClientCerts(t, "./testdata/client.pem", "./testdata/cakey.key", "./testdata/ca.pem")
	require.NoError(t, err)

	conn, err := grpc.DialContext(
		ctx,
		"localhost",
		grpc.WithContextDialer(
			func(ctx context.Context, _ string) (net.Conn, error) {
				return listener.DialContext(ctx)
			},
		),
		tlsDialOpt,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, conn.Close())
	})

	client := pb.NewServiceClient(conn)

	resp, err := client.Start(ctx, &pb.JobStartRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	cancel()

	expectJSON, err := json.Marshal(expect)
	require.NoError(t, err)

	gotJSON, err := json.Marshal(resp)
	require.NoError(t, err)

	assert.JSONEq(t, string(expectJSON), string(gotJSON))
}

func setupClientCerts(t *testing.T, certPath, keyPath, chainPath string) (grpc.DialOption, error) {
	t.Helper()
	cert, certPool, err := setupTestCerts(t, certPath, keyPath, chainPath)
	if err != nil {
		return nil, err
	}

	tlsDialOpt := grpc.WithTransportCredentials(
		credentials.NewTLS(
			&tls.Config{
				ClientAuth:   tls.RequireAndVerifyClientCert,
				Certificates: []tls.Certificate{cert},
				RootCAs:      certPool,
				MinVersion:   tls.VersionTLS13,
			},
		),
	)

	return tlsDialOpt, nil
}

func setupTestCerts(t *testing.T, certPath, keyPath, chainPath string) (tls.Certificate, *x509.CertPool, error) {
	t.Helper()
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("unable to load key pair: %w", err)
	}

	chainReader, err := os.OpenFile(chainPath, os.O_RDONLY, 0444)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("unable to open chain file: %w", err)
	}

	bits, err := io.ReadAll(chainReader)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("unable to read from chain file: %w", err)
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(bits)

	return cert, certPool, nil
}

type skeleton struct {
	pb.UnimplementedServiceServer
	handleStart func(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error)
}

// Start  ...
func (sk *skeleton) Start(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
	return sk.handleStart(ctx, req)
}
