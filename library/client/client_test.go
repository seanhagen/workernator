package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"testing"

	pb "github.com/seanhagen/workernator/internal/pb"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/test/bufconn"
)

// the Go test that runs our suite
func TestClient_Client(t *testing.T) {
	suite.Run(t, new(ClientTestSuite))
}

const bufSize = 1024 * 1024

type ClientTestSuite struct {
	suite.Suite

	client *Client
	server testServer

	td testDial
}

// startServer sets up and runs a grpc server that will be stopped
// when the test is over
func (cts *ClientTestSuite) startServer() {
	mtlsConf, err := setupServerCerts(cts.T(), "./testdata/server.pem", "./testdata/cakey.key", "./testdata/ca.pem")
	cts.Require().NoError(err)

	srv := grpc.NewServer(mtlsConf)
	pb.RegisterServiceServer(srv, &cts.server)

	go func() {
		if err := srv.Serve(cts.td.listener); err != nil {
			cts.Error(err)
		}
	}()

	cts.T().Cleanup(func() {
		srv.Stop()
	})
}

// SetupTest handles any setup required by ALL the tests in this suite
func (cts *ClientTestSuite) SetupTest() {
	cts.td = testDial{
		listener: bufconn.Listen(bufSize),
	}
	cts.server = testServer{}

	conf := Config{
		Host: "localhost",
		Port: "8080",

		CertPath:  "./testdata/client.pem",
		KeyPath:   "./testdata/cakey.key",
		ChainPath: "./testdata/ca.pem",

		DialOpts: []grpc.DialOption{
			grpc.WithContextDialer(cts.td.dialer),
		},
	}

	ctx := context.TODO()
	client, err := NewClient(ctx, conf)
	cts.NoError(err)
	cts.NotNil(client)

	cts.client = client
}

type testDial struct {
	listener *bufconn.Listener
}

// dialer  ...
func (td testDial) dialer(ctx context.Context, _ string) (net.Conn, error) {
	return td.listener.DialContext(ctx)
}

type testServer struct {
	pb.UnimplementedServiceServer

	startHandle func(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error)
}

// Start ...
func (ts *testServer) Start(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
	return ts.startHandle(ctx, req)
}

func setupServerCerts(t *testing.T, certPath, keyPath, chainPath string) (grpc.ServerOption, error) {
	t.Helper()
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to load key pair: %w", err)
	}

	chainReader, err := os.OpenFile(chainPath, os.O_RDONLY, 0444)
	if err != nil {
		return nil, fmt.Errorf("unable to open chain file: %w", err)
	}

	bits, err := io.ReadAll(chainReader)
	if err != nil {
		return nil, fmt.Errorf("unable to read from chain file: %w", err)
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(bits)

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
