package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"

	pb "github.com/seanhagen/workernator/internal/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Client is the API client that wraps the auto-generated GRPC methods
// with something a bit nicer.
type Client struct {
	conn *grpc.ClientConn
	grpc pb.ServiceClient
}

// Config is used by NewClient to build a new Client
type Config struct {
	Host string
	Port string

	CertPath  string
	KeyPath   string
	ChainPath string

	DialOpts []grpc.DialOption
}

// NewClient builds a Client, returning an error if it encounters an
// issue while trying to create the client.
//
// Errors can include:
//  - invalid TLS certificates & key for mTLS
//  - invalid host or port
//  - unable to connect to the GRPC server
func NewClient(ctx context.Context, conf Config) (*Client, error) {
	tlsDialOpt, err := setupTLS(conf)
	if err != nil {
		return nil, fmt.Errorf("unable to setup tls: %w", err)
	}

	dialOpts := []grpc.DialOption{tlsDialOpt}
	dialOpts = append(dialOpts, conf.DialOpts...)

	conn, err := grpc.DialContext(
		ctx,
		conf.Host+":"+conf.Port,
		dialOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to server: %w", err)
	}

	client := &Client{
		conn: conn,
		grpc: pb.NewServiceClient(conn),
	}

	return client, nil
}

// Close closes the client connection to the server and returns
// any error that occured during the process.
func (c *Client) Close() error {
	return c.conn.Close()
}

func setupTLS(conf Config) (grpc.DialOption, error) {
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
	certPool.AppendCertsFromPEM(bits)

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
