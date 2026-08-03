package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	database *database.Database
}

func New(db *database.Database) *Store {
	return &Store{database: db}
}

func (s *Store) Reconcile(ctx context.Context, cfg *config.Config) error {
	return s.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, definition := range cfg.Certificates {
			enabled := definition.Enabled == nil || *definition.Enabled
			renewBefore := definition.RenewBefore.Duration
			if renewBefore == 0 {
				renewBefore = config.DefaultRenewBefore
			}
			keyType := definition.KeyType
			if keyType == "" {
				keyType = config.DefaultKeyType
			}
			domains, err := encodeStrings(definition.Domains)
			if err != nil {
				return err
			}

			model := database.Certificate{Name: definition.Name}
			updates := database.Certificate{
				Domains:            domains,
				KeyType:            string(keyType),
				RenewBeforeSeconds: int64(renewBefore.Seconds()),
				Enabled:            enabled,
				UpdatedAt:          time.Now().UTC(),
			}
			result := tx.Where(&database.Certificate{Name: definition.Name}).
				Assign(updates).
				FirstOrCreate(&model)
			if result.Error != nil {
				return result.Error
			}
		}
		return nil
	})
}

func (s *Store) ListCertificates(ctx context.Context) ([]Certificate, error) {
	var models []database.Certificate
	err := s.database.ORM().WithContext(ctx).
		Where(&database.Certificate{Enabled: true}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "name"}}).
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	certificates := make([]Certificate, 0, len(models))
	for _, model := range models {
		certificate, err := certificateFromModel(model)
		if err != nil {
			return nil, err
		}
		version, err := s.CurrentVersion(ctx, model.Name)
		if err != nil && !NotFound(err) {
			return nil, err
		}
		certificate.CurrentVersion = version
		certificates = append(certificates, certificate)
	}
	return certificates, nil
}

func (s *Store) Certificate(ctx context.Context, name string) (Certificate, error) {
	var model database.Certificate
	err := s.database.ORM().WithContext(ctx).
		Where(&database.Certificate{Name: name, Enabled: true}).
		First(&model).Error
	if err != nil {
		return Certificate{}, err
	}
	certificate, err := certificateFromModel(model)
	if err != nil {
		return Certificate{}, err
	}
	version, err := s.CurrentVersion(ctx, name)
	if err != nil && !NotFound(err) {
		return Certificate{}, err
	}
	certificate.CurrentVersion = version
	return certificate, nil
}

func (s *Store) CurrentVersion(ctx context.Context, name string) (*Version, error) {
	var model database.CertificateVersion
	err := s.database.ORM().WithContext(ctx).
		Where(&database.CertificateVersion{CertificateName: name}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true}).
		First(&model).Error
	if err != nil {
		return nil, err
	}
	version, err := versionFromModel(model)
	return &version, err
}

func (s *Store) Versions(ctx context.Context, name string) ([]Version, error) {
	var models []database.CertificateVersion
	err := s.database.ORM().WithContext(ctx).
		Where(&database.CertificateVersion{CertificateName: name}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true}).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	versions := make([]Version, 0, len(models))
	for _, model := range models {
		version, err := versionFromModel(model)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func (s *Store) AddVersion(ctx context.Context, version Version) error {
	domains, err := encodeStrings(version.Domains)
	if err != nil {
		return err
	}
	model := database.CertificateVersion{
		CertificateName:   version.CertificateName,
		Path:              version.Path,
		Domains:           domains,
		Serial:            version.Serial,
		Issuer:            version.Issuer,
		FingerprintSHA256: version.FingerprintSHA256,
		NotBefore:         version.NotBefore,
		NotAfter:          version.NotAfter,
		CreatedAt:         version.CreatedAt,
	}
	return s.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		return tx.Model(&database.Certificate{}).
			Where(&database.Certificate{Name: version.CertificateName}).
			Updates(map[string]any{
				"status":     "valid",
				"last_error": "",
				"updated_at": time.Now().UTC(),
			}).Error
	})
}

func (s *Store) StartJob(ctx context.Context, name, kind string) (int64, error) {
	model := database.Job{
		CertificateName: name,
		Kind:            kind,
		Status:          "running",
		StartedAt:       time.Now().UTC(),
	}
	if err := s.database.ORM().WithContext(ctx).Create(&model).Error; err != nil {
		return 0, err
	}
	return model.ID, nil
}

func (s *Store) FinishJob(ctx context.Context, id int64, jobErr error) error {
	status := "succeeded"
	message := ""
	if jobErr != nil {
		status = "failed"
		message = jobErr.Error()
	}
	finishedAt := time.Now().UTC()
	return s.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.Job{}).
			Where(&database.Job{ID: id}).
			Updates(map[string]any{
				"status":      status,
				"error":       message,
				"finished_at": finishedAt,
			}).Error; err != nil {
			return err
		}
		if jobErr == nil {
			return nil
		}
		var job database.Job
		if err := tx.First(&job, id).Error; err != nil {
			return err
		}
		return tx.Model(&database.Certificate{}).
			Where(&database.Certificate{Name: job.CertificateName}).
			Updates(map[string]any{
				"status":     "error",
				"last_error": message,
				"updated_at": finishedAt,
			}).Error
	})
}

func (s *Store) Jobs(ctx context.Context, limit int) ([]Job, error) {
	var models []database.Job
	err := s.database.ORM().WithContext(ctx).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(models))
	for _, model := range models {
		jobs = append(jobs, Job{
			ID:              model.ID,
			CertificateName: model.CertificateName,
			Kind:            model.Kind,
			Status:          model.Status,
			Error:           model.Error,
			StartedAt:       model.StartedAt,
			FinishedAt:      model.FinishedAt,
		})
	}
	return jobs, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, name string, scopes, certificates []string, expires *time.Time) (APIKey, string, error) {
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
	encodedCertificates, err := encodeStrings(certificates)
	if err != nil {
		return APIKey{}, "", err
	}
	now := time.Now().UTC()
	model := database.APIKey{
		Name:         name,
		Prefix:       prefix,
		SecretHash:   hex.EncodeToString(hash[:]),
		Scopes:       encodedScopes,
		Certificates: encodedCertificates,
		CreatedAt:    now,
		ExpiresAt:    expires,
	}
	if err := s.database.ORM().WithContext(ctx).Create(&model).Error; err != nil {
		return APIKey{}, "", err
	}
	return APIKey{
		ID:           model.ID,
		Name:         name,
		Prefix:       prefix,
		Scopes:       scopes,
		Certificates: certificates,
		CreatedAt:    now,
		ExpiresAt:    expires,
	}, token, nil
}

func (s *Store) Authenticate(ctx context.Context, token, ip string) (Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Principal{}, errors.New("invalid API key")
	}
	var model database.APIKey
	if err := s.database.ORM().WithContext(ctx).
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
	certificates, err := decodeStrings(model.Certificates)
	if err != nil {
		return Principal{}, err
	}
	now := time.Now().UTC()
	_ = s.database.ORM().WithContext(ctx).Model(&model).Updates(map[string]any{
		"last_used_at": now,
		"last_used_ip": ip,
	}).Error
	return Principal{KeyID: model.ID, Name: model.Name, Scopes: scopes, Certificates: certificates}, nil
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var models []database.APIKey
	err := s.database.ORM().WithContext(ctx).
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
		certificates, err := decodeStrings(model.Certificates)
		if err != nil {
			return nil, err
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

func (s *Store) RevokeAPIKey(ctx context.Context, id int64) error {
	result := s.database.ORM().WithContext(ctx).
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

func (s *Store) Audit(ctx context.Context, actor, action, resource, detail, ip string) {
	_ = s.database.ORM().WithContext(ctx).Create(&database.AuditEvent{
		At:       time.Now().UTC(),
		Actor:    actor,
		Action:   action,
		Resource: resource,
		Detail:   detail,
		IP:       ip,
	}).Error
}

func (s *Store) Audits(ctx context.Context, limit int) ([]Audit, error) {
	var models []database.AuditEvent
	err := s.database.ORM().WithContext(ctx).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	audits := make([]Audit, 0, len(models))
	for _, model := range models {
		audits = append(audits, Audit{
			ID: model.ID, At: model.At, Actor: model.Actor, Action: model.Action,
			Resource: model.Resource, Detail: model.Detail, IP: model.IP,
		})
	}
	return audits, nil
}

func (p Principal) Allows(scope, certificate string) bool {
	if !contains(p.Scopes, scope) && !contains(p.Scopes, "*") {
		return false
	}
	return certificate == "" || contains(p.Certificates, certificate) || contains(p.Certificates, "*")
}

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
