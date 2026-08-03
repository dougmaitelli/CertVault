package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type APIKeyRepository struct {
	database *database.Database
}

func (r *APIKeyRepository) Create(ctx context.Context, name string, scopes, certificates []string, expires *time.Time) (APIKey, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return APIKey{}, "", err
	}
	prefixRaw := make([]byte, 5)
	if _, err := rand.Read(prefixRaw); err != nil {
		return APIKey{}, "", err
	}
	prefix := "cv_live_" + hex.EncodeToString(prefixRaw)
	token := prefix + "." + base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	encodedScopes, err := encodeStrings(scopes)
	if err != nil {
		return APIKey{}, "", err
	}
	allCertificates, certificateNames := normalizeCertificateAccess(certificates)
	now := time.Now().UTC()
	model := database.APIKey{
		Name:            name,
		Prefix:          prefix,
		SecretHash:      hex.EncodeToString(hash[:]),
		Scopes:          encodedScopes,
		AllCertificates: allCertificates,
		CreatedAt:       now,
		ExpiresAt:       expires,
	}
	if err := r.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		for _, certificateName := range certificateNames {
			certificate, err := findCertificate(tx, certificateName)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("certificate %q does not exist", certificateName)
			}
			if err != nil {
				return err
			}
			access := database.APIKeyCertificate{
				APIKeyID:      model.ID,
				CertificateID: certificate.ID,
			}
			if err := tx.Create(&access).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return APIKey{}, "", err
	}
	visibleCertificates := certificateNames
	if allCertificates {
		visibleCertificates = []string{"*"}
	}
	return APIKey{
		ID:           model.ID,
		Name:         name,
		Prefix:       prefix,
		Scopes:       scopes,
		Certificates: visibleCertificates,
		CreatedAt:    now,
		ExpiresAt:    expires,
	}, token, nil
}

func (r *APIKeyRepository) Authenticate(ctx context.Context, token, ip string) (Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Principal{}, errors.New("invalid API key")
	}
	var model database.APIKey
	if err := r.database.ORM().WithContext(ctx).
		Preload("CertificateAccess.Certificate").
		Where(&database.APIKey{Prefix: parts[0]}).
		First(&model).Error; err != nil {
		return Principal{}, errors.New("invalid API key")
	}
	hash := sha256.Sum256([]byte(token))
	if model.SecretHash != hex.EncodeToString(hash[:]) || model.Revoked ||
		(model.ExpiresAt != nil && model.ExpiresAt.Before(time.Now())) {
		return Principal{}, errors.New("invalid API key")
	}
	scopes, err := decodeStrings(model.Scopes)
	if err != nil {
		return Principal{}, err
	}
	certificates := certificateNames(model)
	now := time.Now().UTC()
	_ = r.database.ORM().WithContext(ctx).Model(&model).Updates(map[string]any{
		"last_used_at": now,
		"last_used_ip": ip,
	}).Error
	return Principal{
		KeyID:           model.ID,
		Name:            model.Name,
		Scopes:          scopes,
		Certificates:    certificates,
		AllCertificates: model.AllCertificates,
	}, nil
}

func (r *APIKeyRepository) List(ctx context.Context) ([]APIKey, error) {
	var models []database.APIKey
	err := r.database.ORM().WithContext(ctx).
		Preload("CertificateAccess.Certificate").
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	keys := make([]APIKey, 0, len(models))
	for _, model := range models {
		scopes, err := decodeStrings(model.Scopes)
		if err != nil {
			return nil, err
		}
		certificates := certificateNames(model)
		if model.AllCertificates {
			certificates = []string{"*"}
		}
		keys = append(keys, APIKey{
			ID:           model.ID,
			Name:         model.Name,
			Prefix:       model.Prefix,
			Scopes:       scopes,
			Certificates: certificates,
			CreatedAt:    model.CreatedAt,
			LastUsedAt:   model.LastUsedAt,
			LastUsedIP:   model.LastUsedIP,
			ExpiresAt:    model.ExpiresAt,
			Revoked:      model.Revoked,
		})
	}
	return keys, nil
}

func normalizeCertificateAccess(certificates []string) (bool, []string) {
	unique := make(map[string]struct{}, len(certificates))
	for _, certificate := range certificates {
		if certificate == "*" {
			return true, nil
		}
		unique[certificate] = struct{}{}
	}
	names := make([]string, 0, len(unique))
	for certificate := range unique {
		names = append(names, certificate)
	}
	sort.Strings(names)
	return false, names
}

func certificateNames(key database.APIKey) []string {
	names := make([]string, 0, len(key.CertificateAccess))
	for _, access := range key.CertificateAccess {
		names = append(names, access.Certificate.Name)
	}
	sort.Strings(names)
	return names
}

func (r *APIKeyRepository) Revoke(ctx context.Context, id int64) error {
	result := r.database.ORM().WithContext(ctx).
		Model(&database.APIKey{}).
		Where(&database.APIKey{ID: id}).
		Update("revoked", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
