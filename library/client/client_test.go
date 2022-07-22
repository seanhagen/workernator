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

	listener *bufconn.Listener
}

// startServer sets up and runs a grpc server that will be stopped
// when the test is over
func (cts *ClientTestSuite) startServer() {
	mtlsConf, err := setupServerCerts(cts.T(), "./testdata/server.pem", "./testdata/cakey.key", "./testdata/ca.pem")
	cts.Require().NoError(err)

	srv := grpc.NewServer(mtlsConf)
	pb.RegisterServiceServer(srv, &cts.server)

	go func() {
		if err := srv.Serve(cts.listener); err != nil {
			cts.Error(err)
		}
	}()

	cts.T().Cleanup(func() {
		srv.Stop()
	})
}

// SetupTest handles any setup required by ALL the tests in this suite
func (cts *ClientTestSuite) SetupTest() {
	conn := bufconn.Listen(bufSize)
	cts.server = testServer{}

	conf := Config{
		Host: "localhost",
		Port: "8080",

		CertPath:  "./testdata/client.pem",
		KeyPath:   "./testdata/cakey.key",
		ChainPath: "./testdata/ca.pem",

		DialOpts: []grpc.DialOption{
			grpc.WithContextDialer(
				func(ctx context.Context, _ string) (net.Conn, error) {
					return conn.DialContext(ctx)
				},
			),
		},
	}

	ctx := context.TODO()
	client, err := NewClient(ctx, conf)
	cts.NoError(err)
	cts.NotNil(client)

	cts.listener = conn
	cts.client = client
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

// testServer is the fake grpc server used when testing the client
type testServer struct {
	pb.UnimplementedServiceServer

	startHandle  func(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error)
	stopHandle   func(ctx context.Context, req *pb.JobStopRequest) (*pb.Job, error)
	statusHandle func(ctx context.Context, req *pb.JobStatusRequest) (*pb.Job, error)
	outputHandle func(req *pb.OutputJobRequest, strm pb.Service_OutputServer) error
}

func (ts *testServer) Start(ctx context.Context, req *pb.JobStartRequest) (*pb.Job, error) {
	return ts.startHandle(ctx, req)
}

func (ts *testServer) Stop(ctx context.Context, req *pb.JobStopRequest) (*pb.Job, error) {
	return ts.stopHandle(ctx, req)
}

func (ts *testServer) Status(ctx context.Context, req *pb.JobStatusRequest) (*pb.Job, error) {
	return ts.statusHandle(ctx, req)
}

func (ts *testServer) Output(req *pb.OutputJobRequest, strm pb.Service_OutputServer) error {
	return ts.outputHandle(req, strm)
}
