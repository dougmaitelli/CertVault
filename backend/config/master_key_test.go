package config

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadMasterKeyFromEnvironment(t *testing.T) {
	expected := bytes.Repeat([]byte{42}, 32)
	t.Setenv(EnvMasterKey, base64.StdEncoding.EncodeToString(expected))
	t.Setenv(EnvMasterKeyFile, "")

	actual, err := loadMasterKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("master key = %x, want %x", actual, expected)
	}
}

func TestLoadMasterKeyRejectsValueAndFileTogether(t *testing.T) {
	t.Setenv(EnvMasterKey, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv(EnvMasterKeyFile, "/run/secrets/master_key")

	_, err := loadMasterKey(t.TempDir())
	if err == nil {
		t.Fatal("loadMasterKey accepted both master key variables")
	}
	if !strings.Contains(err.Error(), EnvMasterKey) ||
		!strings.Contains(err.Error(), EnvMasterKeyFile) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeMasterKeyRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"not-base64", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		if _, err := decodeMasterKey(value); err == nil {
			t.Fatalf("decodeMasterKey(%q) succeeded", value)
		}
	}
}
