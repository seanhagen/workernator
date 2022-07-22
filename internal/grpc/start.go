package grpc

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

// Start launches the GRPC server so it can handle incoming requests.
func (s *Server) Start(ctx context.Context) error {
	zap.L().Info("launching grpc server", zap.String("port", s.config.Port))

	errChan := make(chan error)
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGABRT)
	defer cancel()

	go s.runGRPC(ctx, errChan)

	select {
	case <-ctx.Done():
		zap.L().Info("server stopping")
	case err := <-errChan:
		zap.L().Error("server encountered error during runtime, shutting down", zap.Error(err))
	}

	return s.stop()
}

func (s *Server) runGRPC(ctx context.Context, errChan chan<- error) {
	if s.srv == nil {
		errChan <- fmt.Errorf("server hasn't been intialized properly")
		return
	}

	if err := s.srv.Serve(s.listen); err != nil {
		errChan <- err
	}

	zap.L().Info("grpc server stopped")
}

// stop is where any pre-shutdown things should get handled, such as
// syncing any logs to disk, etc. Right now there isn't anything that
// needs to happen here, so this is more of a placeholder for later.
func (s *Server) stop() error {
	s.srv.Stop()
	return nil
}
