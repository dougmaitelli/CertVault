package config

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMasterKeyFromEnvironment(t *testing.T) {
	expected := bytes.Repeat([]byte{42}, 32)
	t.Setenv("CERTVAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(expected))
	t.Setenv("CERTVAULT_MASTER_KEY_FILE", "")

	configuration := Config{}
	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(configuration.MasterKey, expected) {
		t.Fatalf("master key = %x, want %x", configuration.MasterKey, expected)
	}
}

func TestLoadMasterKeyFromFileEnvironment(t *testing.T) {
	expected := bytes.Repeat([]byte{42}, 32)

	path := filepath.Join(t.TempDir(), "master-key")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(expected)), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CERTVAULT_MASTER_KEY", "")
	t.Setenv("CERTVAULT_MASTER_KEY_FILE", path)

	configuration := Config{}
	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(configuration.MasterKey, expected) {
		t.Fatalf("master key = %x, want %x", configuration.MasterKey, expected)
	}
}

func TestLoadMasterKeyRejectsValueAndFileTogether(t *testing.T) {
	t.Setenv("CERTVAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("CERTVAULT_MASTER_KEY_FILE", "/run/secrets/master_key")

	err := applyEnv(&Config{})
	if err == nil {
		t.Fatal("applyEnv accepted both master key variables")
	}

	if !strings.Contains(err.Error(), "CERTVAULT_MASTER_KEY") ||
		!strings.Contains(err.Error(), "CERTVAULT_MASTER_KEY_FILE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeMasterKeyRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"not-base64", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		var key MasterKey
		if err := key.UnmarshalText([]byte(value)); err == nil {
			t.Fatalf("MasterKey.UnmarshalText(%q) succeeded", value)
		}
	}
}
