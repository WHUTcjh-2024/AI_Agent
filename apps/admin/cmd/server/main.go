package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"asku/admin"
	"asku/admin/internal/console"
)

func main() {
	if err := serve(); err != nil {
		slog.Error("admin stopped", "error", err)
		os.Exit(1)
	}
}

func serve() error {
	cfg, err := console.LoadConfig()
	if err != nil {
		return err
	}
	files, err := fs.Sub(admin.Assets, "web")
	if err != nil {
		return err
	}
	handler, err := console.New(cfg, files, nil)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failed := make(chan error, 1)
	go func() { slog.Info("AskU admin started", "addr", cfg.Addr); failed <- server.ListenAndServe() }()
	select {
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
	return nil
}
