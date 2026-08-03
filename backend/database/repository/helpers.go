package repository

import (
	"encoding/json"
	"errors"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
)

func NotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func certificateFromModel(model database.Certificate) (Certificate, error) {
	domains, err := decodeStrings(model.Domains)
	if err != nil {
		return Certificate{}, err
	}
	return Certificate{
		Name: model.Name, Domains: domains, KeyType: config.KeyType(model.KeyType), Status: model.Status,
		RenewBeforeSeconds: model.RenewBeforeSeconds, LastError: model.LastError,
	}, nil
}

func versionFromModel(model database.CertificateVersion) (Version, error) {
	domains, err := decodeStrings(model.Domains)
	if err != nil {
		return Version{}, err
	}
	return Version{
		ID: model.ID, CertificateName: model.CertificateName, Path: model.Path,
		Serial: model.Serial, Issuer: model.Issuer, FingerprintSHA256: model.FingerprintSHA256,
		NotBefore: model.NotBefore, NotAfter: model.NotAfter, CreatedAt: model.CreatedAt, Domains: domains,
	}, nil
}

func encodeStrings(values []string) (string, error) {
	encoded, err := json.Marshal(values)
	return string(encoded), err
}

func decodeStrings(value string) ([]string, error) {
	var values []string
	err := json.Unmarshal([]byte(value), &values)
	return values, err
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
