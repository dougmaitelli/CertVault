package service

import (
	"context"
	"testing"

	"github.com/certvault/certvault/config"
)

func TestIssueRejectsUnknownCertificateBeforeCreatingLock(t *testing.T) {
	manager, err := NewManager(&config.Config{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err = manager.Issue(context.Background(), "unknown", "manual"); err == nil {
		t.Fatal("expected unknown certificate error")
	}
	if _, exists := manager.locks.Load("unknown"); exists {
		t.Fatal("unknown certificate created an issuance lock")
	}
}
