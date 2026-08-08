package config

import (
	"strings"
)

// KeyType identifies the private-key algorithm and size used for a certificate.
type KeyType string

const (
	KeyTypeEC256   KeyType = "ec256"
	KeyTypeEC384   KeyType = "ec384"
	KeyTypeRSA2048 KeyType = "rsa2048"
	KeyTypeRSA3072 KeyType = "rsa3072"
	KeyTypeRSA4096 KeyType = "rsa4096"

	DefaultKeyType = KeyTypeEC256
)

// UnmarshalText normalizes a configured key type before validation.
func (k *KeyType) UnmarshalText(text []byte) error {
	*k = KeyType(strings.ToLower(strings.TrimSpace(string(text))))

	return nil
}

// Valid reports whether the key type is supported by CertVault.
func (k KeyType) Valid() bool {
	switch k {
	case KeyTypeEC256, KeyTypeEC384, KeyTypeRSA2048, KeyTypeRSA3072, KeyTypeRSA4096:
		return true
	default:
		return false
	}
}
