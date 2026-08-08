package config

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestKeyTypeUnmarshalYAML(t *testing.T) {
	var value struct {
		KeyType KeyType `yaml:"key_type"`
	}
	if err := yaml.Unmarshal([]byte("key_type: EC384\n"), &value); err != nil {
		t.Fatal(err)
	}

	if value.KeyType != KeyTypeEC384 {
		t.Fatalf("key type = %q, want %q", value.KeyType, KeyTypeEC384)
	}
}

func TestKeyTypeValid(t *testing.T) {
	for _, keyType := range []KeyType{
		KeyTypeEC256,
		KeyTypeEC384,
		KeyTypeRSA2048,
		KeyTypeRSA3072,
		KeyTypeRSA4096,
	} {
		if !keyType.Valid() {
			t.Errorf("expected %q to be valid", keyType)
		}
	}

	if KeyType("unsupported").Valid() {
		t.Fatal("unsupported key type reported as valid")
	}
}
