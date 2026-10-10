package service

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/go-acme/lego/v5/certificate"
)

type certificateArtifacts struct {
	certificate *x509.Certificate
	leaf        []byte
	chain       []byte
	fullChain   []byte
}

func normalizeCertificateArtifacts(resource *certificate.Resource) (certificateArtifacts, error) {
	if resource == nil {
		return certificateArtifacts{}, errors.New("ACME response contained no certificate")
	}

	certificates, err := parseCertificatePEM(resource.Certificate)
	if err != nil {
		return certificateArtifacts{}, fmt.Errorf("parse ACME certificate: %w", err)
	}

	if len(certificates) == 0 {
		return certificateArtifacts{}, errors.New("ACME response contained no certificate")
	}

	leaf := certificates[0]
	issuers := certificates[1:]
	// The bundled chain is authoritative. The separate issuer field repeats that
	// chain in Lego; use it only when the certificate field contains just a leaf.
	if len(issuers) == 0 {
		issuers, err = parseCertificatePEM(resource.IssuerCertificate)
		if err != nil {
			return certificateArtifacts{}, fmt.Errorf("parse ACME issuer chain: %w", err)
		}
	}

	artifacts := certificateArtifacts{certificate: leaf, leaf: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})}

	seen := map[string]bool{string(leaf.Raw): true}
	for _, issuer := range issuers {
		if seen[string(issuer.Raw)] {
			continue
		}

		seen[string(issuer.Raw)] = true
		artifacts.chain = append(artifacts.chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: issuer.Raw})...)
	}

	artifacts.fullChain = append(append([]byte(nil), artifacts.leaf...), artifacts.chain...)

	return artifacts, nil
}

func parseCertificatePEM(data []byte) ([]*x509.Certificate, error) {
	var certificates []*x509.Certificate

	for len(bytes.TrimSpace(data)) > 0 {
		data = bytes.TrimSpace(data)
		if !bytes.HasPrefix(data, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, errors.New("expected certificate PEM block")
		}

		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("invalid certificate PEM block")
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}

		certificates = append(certificates, cert)
		data = rest
	}

	return certificates, nil
}
