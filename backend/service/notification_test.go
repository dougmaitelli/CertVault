package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/certvault/certvault/config"
)

func TestAppriseNotifierPostsDockDashCompatiblePayload(t *testing.T) {
	var received apprisePayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}

		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Errorf("content type = %q", contentType)
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode payload: %v", err)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	n := newAppriseNotifier(config.Notifications{
		AppriseURL:  server.URL,
		AppriseURLs: []string{"discord://example/token"},
		AppriseTags: []string{"admin", "homelab"},
	})

	if err := n.Notify(context.Background(), "Certificate renewed", "home renewed", NotificationSuccess); err != nil {
		t.Fatal(err)
	}

	if received.Title != "CertVault — Certificate renewed" ||
		received.Body != "home renewed" || received.Type != NotificationSuccess {
		t.Fatalf("payload = %#v", received)
	}

	if !slices.Equal(received.URLs, []string{"discord://example/token"}) {
		t.Fatalf("urls = %#v", received.URLs)
	}

	if !slices.Equal(received.Tags, []string{"admin", "homelab"}) {
		t.Fatalf("tags = %#v", received.Tags)
	}
}

func TestAppriseNotifierIsNoOpWithoutURL(t *testing.T) {
	n := newAppriseNotifier(config.Notifications{})
	if n.Configured() {
		t.Fatal("empty notifier is configured")
	}

	if err := n.Notify(context.Background(), "title", "body", NotificationInfo); err != nil {
		t.Fatal(err)
	}
}

func TestAppriseNotifierReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "configuration not found", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	n := newAppriseNotifier(config.Notifications{AppriseURL: server.URL})

	err := n.Notify(context.Background(), "title", "body", NotificationWarning)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") ||
		!strings.Contains(err.Error(), "configuration not found") {
		t.Fatalf("error = %v", err)
	}
}
