package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certvault/certvault/audit"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
)

func TestAPIKeyLifecycle(t *testing.T) {
	configPath, dataDir := writeTestConfig(t)

	var (
		output bytes.Buffer
		errors bytes.Buffer
	)

	err := RunAPIKey([]string{
		"create",
		"--config", configPath,
		"--name", "traefik",
		"--scope", "certificates:read",
		"--scope", "private_keys:read",
		"--certificate", "*",
	}, &output, &errors)
	if err != nil {
		t.Fatalf("create API key: %v (%s)", err, errors.String())
	}

	token := strings.TrimSpace(output.String())
	if !strings.HasPrefix(token, "cv_live_") || strings.Count(token, ".") != 1 {
		t.Fatalf("created token = %q", token)
	}

	output.Reset()

	if err = RunAPIKey(
		[]string{"list", "--config", configPath}, &output, &errors,
	); err != nil {
		t.Fatalf("list API keys: %v", err)
	}

	var keys []repository.APIKey
	if err = json.Unmarshal(output.Bytes(), &keys); err != nil {
		t.Fatalf("decode listed API keys: %v", err)
	}

	if len(keys) != 1 || keys[0].Name != "traefik" || keys[0].ID != 1 {
		t.Fatalf("listed API keys = %#v", keys)
	}

	output.Reset()

	if err = RunAPIKey(
		[]string{"revoke", "--config", configPath, "--id", "1"}, &output, &errors,
	); err != nil {
		t.Fatalf("revoke API key: %v", err)
	}

	if output.String() != "revoked API key 1 (traefik)\n" {
		t.Fatalf("revoke output = %q", output.String())
	}

	output.Reset()

	if err = RunAPIKey(
		[]string{"delete", "--config", configPath, "--id", "1"}, &output, &errors,
	); err != nil {
		t.Fatalf("delete API key: %v", err)
	}

	if output.String() != "deleted API key 1 (traefik)\n" {
		t.Fatalf("delete output = %q", output.String())
	}

	db, err := database.Open(filepath.Join(dataDir, "certvault.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	audits, err := repository.New(db).Audits.Search(context.Background(), repository.AuditFilter{
		Actors:  []string{audit.ActorLocalCLI},
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if audits.Total != 3 {
		t.Fatalf("CLI audit events = %d, want 3", audits.Total)
	}
}

func TestCreateAPIKeyValidatesRequiredOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "name", args: []string{"create"}, want: "--name is required"},
		{
			name: "scope",
			args: []string{"create", "--name", "client"},
			want: "at least one --scope is required",
		},
		{
			name: "known scope",
			args: []string{"create", "--name", "client", "--scope", "admin"},
			want: `unsupported scope "admin"`,
		},
		{
			name: "certificate",
			args: []string{"create", "--name", "client", "--scope", "certificates:read"},
			want: "at least one --certificate is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := RunAPIKey(test.args, &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func writeTestConfig(t *testing.T) (string, string) {
	t.Helper()
	dataDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	contents := "data_dir: " + dataDir + "\n" +
		"acme:\n" +
		"  email: test@example.com\n" +
		"  accept_terms: true\n"
	if err := os.WriteFile(configPath, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.EnvMasterKey, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv(config.EnvMasterKeyFile, "")

	return configPath, dataDir
}
