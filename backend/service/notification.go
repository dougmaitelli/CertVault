package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/certvault/certvault/config"
)

type NotificationType string

const (
	NotificationInfo    NotificationType = "info"
	NotificationSuccess NotificationType = "success"
	NotificationWarning NotificationType = "warning"
	NotificationFailure NotificationType = "failure"
	notificationTimeout                  = 5 * time.Second
	maxAppriseErrorBody                  = 4 << 10
)

type notifier interface {
	Configured() bool
	Notify(context.Context, string, string, NotificationType) error
}

type appriseNotifier struct {
	url    string
	urls   []string
	tags   []string
	client *http.Client
}

type apprisePayload struct {
	Title string           `json:"title"`
	Body  string           `json:"body"`
	Type  NotificationType `json:"type"`
	URLs  []string         `json:"urls,omitempty"`
	Tags  []string         `json:"tag,omitempty"`
}

func newAppriseNotifier(cfg config.Notifications) notifier {
	return &appriseNotifier{
		url:    cfg.AppriseURL,
		urls:   cfg.AppriseURLs,
		tags:   cfg.AppriseTags,
		client: &http.Client{Timeout: notificationTimeout},
	}
}

func (n *appriseNotifier) Configured() bool {
	return n.url != ""
}

func (n *appriseNotifier) Notify(
	ctx context.Context,
	title string,
	body string,
	typeName NotificationType,
) error {
	if !n.Configured() {
		return nil
	}

	payload, err := json.Marshal(apprisePayload{
		Title: "CertVault — " + title,
		Body:  body,
		Type:  typeName,
		URLs:  n.urls,
		Tags:  n.tags,
	})
	if err != nil {
		return fmt.Errorf("encode Apprise notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create Apprise request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send Apprise notification: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxAppriseErrorBody))

		detail := strings.TrimSpace(string(bodyBytes))
		if detail == "" {
			return fmt.Errorf("send Apprise notification: HTTP %s", resp.Status)
		}

		return fmt.Errorf("send Apprise notification: HTTP %s: %s", resp.Status, detail)
	}

	return nil
}
