package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dshare/internal/app"
	"dshare/internal/config"
)

func main() {
	cfg := config.Load()
	serverApp, err := app.New(cfg)
	if err != nil {
		log.Fatalf("start app: %v", err)
	}
	defer serverApp.Close()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           serverApp.Routes(),
		ReadHeaderTimeout: 15 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("dshare listening on %s", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
