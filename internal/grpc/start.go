package grpc

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// Start  ...
func (s *Server) Start(ctx context.Context) error {
	fmt.Printf("Launching GRPC server on port '%v'\n", s.config.Port)

	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGABRT)
	defer signal.Stop(sigChan)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go s.runGRPC(ctx, errChan)

	select {
	case <-sigChan:
		fmt.Printf("got signal to exit!\n")
	case err := <-errChan:
		fmt.Printf("server encountered error during runtime: %v\n", err)
	}

	fmt.Printf("Server stopped!\n")
	return s.stop(ctx)
}

// runGRPC  ...
func (s *Server) runGRPC(ctx context.Context, errChan chan<- error) {
	if s.srv == nil {
		errChan <- fmt.Errorf("server hasn't been intialized properly")
		return
	}

	done := make(chan bool)

	go func() {
		if err := s.srv.Serve(s.listen); err != nil {
			errChan <- err
			done <- true
		}
	}()

	for {
		select {
		case <-ctx.Done():
			break
		case <-done:
			break
		}
	}

	fmt.Printf("GRPC server stopped\n")
}

// stop ...
func (s *Server) stop(ctx context.Context) error {
	return nil
}
