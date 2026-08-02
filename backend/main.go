package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/certvault/certvault/api"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/service"
	"github.com/certvault/certvault/store"
)

func main() {
	configPath := flag.String("config", envOr("CERTVAULT_CONFIG", "/config/config.yaml"), "configuration file")
	check := flag.Bool("check-config", false, "validate configuration and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("load configuration", err)
	}
	if *check {
		fmt.Println("configuration valid")
		return
	}

	level := slog.LevelInfo
	if cfg.Server.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		fatal("create data directory", err)
	}

	db, err := store.Open(filepath.Join(cfg.DataDir, "certvault.db"))
	if err != nil {
		fatal("open database", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Reconcile(context.Background(), cfg); err != nil {
		fatal("reconcile configuration", err)
	}

	manager, err := service.NewManager(cfg, db, log)
	if err != nil {
		fatal("initialize manager", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go manager.Run(ctx)

	handler, err := api.New(cfg, db, manager, log)
	if err != nil {
		fatal("initialize API", err)
	}
	server := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		log.Info("server listening", "address", cfg.Server.Listen)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("serve", err)
		}
	}()
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	_ = server.Shutdown(shutdown)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(msg string, err error) { fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err); os.Exit(1) }
