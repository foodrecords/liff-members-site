package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/foodrecords/members-api/pkg/logger"
)

const (
	timeout = 60 * time.Second
)

func Run(port int, r http.Handler) int {
	errCh := make(chan error, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	s := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           r,
		ReadTimeout:       timeout,
		ReadHeaderTimeout: timeout,
		WriteTimeout:      timeout,
		IdleTimeout:       timeout,
	}

	go func() {
		logger.Info("server listening on port %d", port)
		errCh <- s.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		logger.Fatal(err.Error())
		return 1
	case <-sigCh:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			panic(err)
		}
		return 0
	}
}
