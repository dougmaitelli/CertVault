package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

// runServer loads CertVault and serves requests until it receives a shutdown signal.
func runServer(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("certvault server", flag.ContinueOnError)
	flags.SetOutput(stderr)

	configPath := flags.String("config", config.Path(), "configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	log := slog.New(slog.NewJSONHandler(
		stdout,
		&slog.HandlerOptions{Level: cfg.Server.LogLevel.Level()},
	))

	db, err := database.Open(filepath.Join(cfg.DataDir, "certvault.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	defer func() { _ = db.Close() }()

	repositories := repository.New(db)
	if err := repositories.Certificates.Reconcile(context.Background(), cfg); err != nil {
		return fmt.Errorf("reconcile configuration: %w", err)
	}

	manager, err := service.NewManager(cfg, repositories, log)
	if err != nil {
		return fmt.Errorf("initialize manager: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if cfg.HasAutomaticIssuance() {
		go manager.Run(ctx)
	} else {
		log.Info("automatic certificate issuance disabled")
	}

	if cfg.Audit.Retention.Duration > 0 {
		go service.RunAuditRetention(ctx, repositories.Audits, cfg.Audit.Retention.Duration, log)
	}

	handler, err := api.New(cfg, db, repositories, manager)
	if err != nil {
		return fmt.Errorf("initialize API: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
	serveErrors := make(chan error, 1)

	go func() {
		log.Info("server listening", "address", cfg.Server.Listen)

		if serveErr := server.ListenAndServe(); serveErr != nil &&
			!errors.Is(serveErr, http.ErrServerClosed) {
			serveErrors <- serveErr
		}
	}()

	select {
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer stop()

		_ = server.Shutdown(shutdown)

		return nil
	case serveErr := <-serveErrors:
		return fmt.Errorf("serve: %w", serveErr)
	}
}
