package config

import (
	"strings"
	"testing"
	"time"
)

func TestNegativeAuditRetentionIsRejected(t *testing.T) {
	cfg := Config{
		ACME: ACME{
			Email:        "admin@example.com",
			DirectoryURL: "https://acme.example/directory",
			AcceptTerms:  true,
		},
		Audit: Audit{Retention: Duration{Duration: -time.Hour}},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "audit.retention") {
		t.Fatalf("negative audit retention error = %v", err)
	}
}
