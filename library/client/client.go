package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"time"

	pb "github.com/seanhagen/workernator/internal/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// JobStatus ...
type JobStatus int

const (
	// Unknown ...
	Unknown JobStatus = 0
	// Running ...
	Running = 1
	// Failed ...
	Failed = 2
	// Finished ...
	Finished = 3
	// Stopped ...
	Stopped = 4
)

// JobResponse ...
type JobResponse struct {
	ID      string
	Status  JobStatus
	Cmd     string
	Args    []string
	Err     error
	Started time.Time
	Ended   time.Time
}

// Client ...
type Client struct {
	conn *grpc.ClientConn
	grpc pb.ServiceClient
}

// Config ...
type Config struct {
	Host string
	Port string

	CertPath  string
	KeyPath   string
	ChainPath string

	DialOpts []grpc.DialOption
}

// NewClient ...
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
			},
		),
	)

	return tlsDialOpt, nil
}