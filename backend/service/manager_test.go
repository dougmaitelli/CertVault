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

func TestHookMatchesCertificateAndEvent(t *testing.T) {
	hook := config.Hook{Events: []string{"certificate.renewed"}}
	if !hookMatches(hook, "certificate.renewed", "home") {
		t.Fatal("hook without certificate filter did not match")
	}

	hook.Certificates = []string{"home", "proxy"}
	if !hookMatches(hook, "certificate.renewed", "home") {
		t.Fatal("hook did not match selected certificate")
	}

	if hookMatches(hook, "certificate.renewed", "other") {
		t.Fatal("hook matched unselected certificate")
	}

	if hookMatches(hook, "certificate.failed", "home") {
		t.Fatal("hook matched unselected event")
	}
}
