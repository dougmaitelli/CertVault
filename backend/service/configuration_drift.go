package service

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database/repository"
)

// Compare the exact SAN set: wildcard coverage does not replace an explicitly
// requested SAN, and removing a SAN must also trigger issuance.
func (m *Manager) configurationDrift(c repository.Certificate) (bool, error) {
	v := c.CurrentVersion
	if v == nil {
		return false, nil
	}

	if !sameDomains(c.Domains, v.Domains) {
		return true, nil
	}

	keyType := v.KeyType
	if keyType == "" {
		// Versions created before key-type metadata was added must be inspected,
		// rather than assuming their key matches the current desired configuration.
		data, err := m.ReadFile(v, "certificate.crt")
		if err != nil {
			return false, err
		}

		block, _ := pem.Decode(data)
		if block == nil {
			return false, errors.New("stored certificate has no PEM block")
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return false, err
		}

		keyType = issuedKeyType(cert)
	}

	if keyType == "" {
		return false, fmt.Errorf("unsupported issued public key for %q", c.Name)
	}

	return keyType != c.KeyType, nil
}

func sameDomains(a, b []string) bool {
	normalize := func(domains []string) map[string]struct{} {
		set := make(map[string]struct{}, len(domains))
		for _, domain := range domains {
			set[strings.ToLower(domain)] = struct{}{}
		}

		return set
	}

	left, right := normalize(a), normalize(b)
	if len(left) != len(right) {
		return false
	}

	for domain := range left {
		if _, ok := right[domain]; !ok {
			return false
		}
	}

	return true
}

func issuedKeyType(cert *x509.Certificate) config.KeyType {
	switch key := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		switch key.Curve.Params().BitSize {
		case 256:
			return config.KeyTypeEC256
		case 384:
			return config.KeyTypeEC384
		}
	case *rsa.PublicKey:
		return config.KeyType(fmt.Sprintf("rsa%d", key.N.BitLen()))
	}

	return ""
}
