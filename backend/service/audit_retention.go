package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/certvault/certvault/database/repository"
)

const auditCleanupInterval = 24 * time.Hour

func RunAuditRetention(ctx context.Context, audits *repository.AuditRepository, retention time.Duration, log *slog.Logger) {
	cleanup := func() {
		deleted, err := audits.DeleteBefore(ctx, time.Now().UTC().Add(-retention))
		if err != nil {
			log.Error("clean old audit events", "error", err)
			return
		}

		if deleted > 0 {
			log.Info("cleaned old audit events", "deleted", deleted)
		}
	}

	cleanup()

	ticker := time.NewTicker(auditCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}
