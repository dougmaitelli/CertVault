package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAutomaticIssuanceToEnabled(t *testing.T) {
	configuration := loadAutomaticIssuanceConfig(t, "")
	if !configuration.ACME.AutomaticIssuance {
		t.Fatal("automatic issuance is disabled by default")
	}
}

func TestLoadAllowsAutomaticIssuanceToBeDisabled(t *testing.T) {
	configuration := loadAutomaticIssuanceConfig(t, "  automatic_issuance: false\n")
	if configuration.ACME.AutomaticIssuance {
		t.Fatal("automatic issuance is enabled")
	}
}

func TestCertificateAutomaticIssuanceOverridesGlobalSetting(t *testing.T) {
	enabled := true
	disabled := false
	configuration := Config{
		ACME: ACME{AutomaticIssuance: false},
		Certificates: []Certificate{
			{Name: "inherited"},
			{Name: "enabled", AutomaticIssuance: &enabled},
			{Name: "disabled", AutomaticIssuance: &disabled},
		},
	}

	if configuration.ShouldAutomaticallyIssue(configuration.Certificates[0]) {
		t.Fatal("inherited certificate ignored disabled global setting")
	}
	if !configuration.ShouldAutomaticallyIssue(configuration.Certificates[1]) {
		t.Fatal("certificate override did not enable automatic issuance")
	}
	if configuration.ShouldAutomaticallyIssue(configuration.Certificates[2]) {
		t.Fatal("certificate override did not disable automatic issuance")
	}
	if !configuration.HasAutomaticIssuance() {
		t.Fatal("enabled certificate override did not start scheduler")
	}
}

func loadAutomaticIssuanceConfig(t *testing.T, setting string) *Config {
	t.Helper()
	t.Setenv(EnvMasterKey, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv(EnvMasterKeyFile, "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := "data_dir: " + t.TempDir() + "\n" +
		"acme:\n" +
		"  email: admin@example.com\n" +
		"  accept_terms: true\n" +
		setting
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return configuration
}
