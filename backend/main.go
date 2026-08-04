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
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	"github.com/certvault/certvault/service"
)

const (
	httpReadHeaderTimeout = 10 * time.Second
	httpReadTimeout       = 30 * time.Second
	httpWriteTimeout      = 2 * time.Minute
	httpIdleTimeout       = 2 * time.Minute
	httpShutdownTimeout   = 15 * time.Second
)

func main() {
	configPath := flag.String("config", envOr(config.EnvConfigFile, "/config/config.yaml"), "configuration file")
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

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.Server.LogLevel.Level()}))

	db, err := database.Open(filepath.Join(cfg.DataDir, "certvault.db"))
	if err != nil {
		fatal("open database", err)
	}
	defer func() { _ = db.Close() }()
	repositories := repository.New(db)
	if err := repositories.Certificates.Reconcile(context.Background(), cfg); err != nil {
		fatal("reconcile configuration", err)
	}

	manager, err := service.NewManager(cfg, repositories, log)
	if err != nil {
		fatal("initialize manager", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if cfg.HasAutomaticIssuance() {
		go manager.Run(ctx)
	} else {
		log.Info("automatic certificate issuance disabled")
	}

	handler, err := api.New(cfg, db, repositories, manager)
	if err != nil {
		fatal("initialize API", err)
	}
	server := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
	go func() {
		log.Info("server listening", "address", cfg.Server.Listen)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("serve", err)
		}
	}()
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), httpShutdownTimeout)
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
