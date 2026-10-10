package service

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/go-acme/lego/v5/certificate"
)

func testCertificateChain(t *testing.T) (*certificate.Resource, [][]byte) {
	t.Helper()

	var (
		parent    *x509.Certificate
		parentKey *ecdsa.PrivateKey
	)

	blocks := make([][]byte, 3)

	var private []byte

	for i, name := range []string{"root", "intermediate", "leaf"} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}

		template := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 1)), Subject: pkix.Name{CommonName: name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour), BasicConstraintsValid: true, IsCA: i < 2, KeyUsage: x509.KeyUsageDigitalSignature}
		if template.IsCA {
			template.KeyUsage |= x509.KeyUsageCertSign
		} else {
			template.DNSNames = []string{"example.test"}
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}

		if parent == nil {
			parent = template
			parentKey = key
		}

		der, err := x509.CreateCertificate(rand.Reader, template, parent, key.Public(), parentKey)
		if err != nil {
			t.Fatal(err)
		}

		blocks[2-i] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

		parent, err = x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}

		parentKey = key
		if i == 2 {
			keyDER, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}

			private = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		}
	}

	return &certificate.Resource{Certificate: bytes.Join(blocks, nil), IssuerCertificate: bytes.Join(blocks[1:], nil), PrivateKey: private}, blocks
}

func TestSaveNormalizesBundledCertificateArtifacts(t *testing.T) {
	resource, blocks := testCertificateChain(t)

	leaf, err := parseCertificatePEM(blocks[0])
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name                string
		certificate, issuer []byte
		count               int
	}{
		{"Lego bundled response", resource.Certificate, resource.IssuerCertificate, 3},
		{"leaf-only response", blocks[0], resource.IssuerCertificate, 3},
		{"bundled chain without separate issuer", resource.Certificate, nil, 3},
		{"bundle authoritative over alternate issuer", resource.Certificate, blocks[2], 3},
		{"repeated certificates", bytes.Join([][]byte{blocks[0], blocks[1], blocks[1], blocks[0], blocks[2]}, nil), resource.IssuerCertificate, 3},
		{"leaf without issuer", blocks[0], nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manager{cfg: &config.Config{DataDir: t.TempDir(), MasterKey: make([]byte, 32)}}
			input := &certificate.Resource{Certificate: tc.certificate, IssuerCertificate: tc.issuer, PrivateKey: resource.PrivateKey}

			v, err := m.save("home", input)
			if err != nil {
				t.Fatal(err)
			}

			expected := map[string][]byte{"certificate.crt": blocks[0], "chain.crt": bytes.Join(blocks[1:tc.count], nil), "fullchain.crt": bytes.Join(blocks[:tc.count], nil), "private.key": resource.PrivateKey}
			for file, want := range expected {
				got, err := m.ReadFile(&v, file)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("%s does not match normalized artifact: %v", file, err)
				}
			}

			sum := sha256.Sum256(leaf[0].Raw)
			if v.Serial != leaf[0].SerialNumber.String() || v.FingerprintSHA256 != hex.EncodeToString(sum[:]) || v.KeyType != config.KeyTypeEC256 || len(v.Domains) != 1 || v.Domains[0] != "example.test" {
				t.Fatalf("metadata is not from leaf: %#v", v)
			}

			if tc.count == 3 {
				roots := x509.NewCertPool()
				roots.AppendCertsFromPEM(blocks[2])

				intermediates := x509.NewCertPool()
				intermediates.AppendCertsFromPEM(blocks[1])

				if _, err = leaf[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: "example.test"}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestSaveRejectsMalformedCertificateArtifactsBeforeWriting(t *testing.T) {
	resource, blocks := testCertificateChain(t)
	for _, tc := range []struct {
		name                string
		certificate, issuer []byte
	}{
		{"empty", nil, nil},
		{"invalid leaf", []byte("not PEM"), nil},
		{"invalid DER", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")}), nil},
		{"invalid bundled chain", append(bytes.Clone(blocks[0]), []byte("invalid tail")...), nil},
		{"invalid issuer", blocks[0], []byte("invalid issuer")},
		{"private key in certificate bundle", append(bytes.Clone(blocks[0]), resource.PrivateKey...), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			m := &Manager{cfg: &config.Config{DataDir: dir, MasterKey: make([]byte, 32)}}
			if _, err := m.save("home", &certificate.Resource{Certificate: tc.certificate, IssuerCertificate: tc.issuer, PrivateKey: resource.PrivateKey}); err == nil {
				t.Fatal("malformed response accepted")
			}

			if _, err := os.Stat(filepath.Join(dir, "certificates")); !os.IsNotExist(err) {
				t.Fatalf("malformed response wrote artifacts: %v", err)
			}
		})
	}
}
