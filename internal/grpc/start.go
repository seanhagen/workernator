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

	sigChan := make(chan os.Signal, 1)
	errChan := make(chan error)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGABRT)
	defer signal.Stop(sigChan)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go s.runGRPC(ctx, errChan)

	select {
	case <-sigChan:
		zap.L().Info("received signal to exit")
	case err := <-errChan:
		zap.L().Error("server encountered error during runtime, shutting down", zap.Error(err))
	}

	zap.L().Info("server stopping")
	return s.stop(ctx)
}

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
			goto done
		case <-done:
			goto done
		}
	}

done:
	zap.L().Info("grpc server stopped")
}

// stop is where any pre-shutdown things should get handled, such as
// syncing any logs to disk, etc. Right now there isn't anything that
// needs to happen here, so this is more of a placeholder for later.
func (s *Server) stop(ctx context.Context) error {
	return nil
}
