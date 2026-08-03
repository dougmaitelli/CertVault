package repository

import (
	"context"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CertificateRepository struct {
	database *database.Database
}

func (r *CertificateRepository) Reconcile(ctx context.Context, cfg *config.Config) error {
	return r.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

func (r *CertificateRepository) List(ctx context.Context) ([]Certificate, error) {
	var models []database.Certificate
	err := r.database.ORM().WithContext(ctx).
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
		version, err := r.CurrentVersion(ctx, model.Name)
		if err != nil && !NotFound(err) {
			return nil, err
		}
		certificate.CurrentVersion = version
		certificates = append(certificates, certificate)
	}
	return certificates, nil
}

func (r *CertificateRepository) Get(ctx context.Context, name string) (Certificate, error) {
	var model database.Certificate
	err := r.database.ORM().WithContext(ctx).
		Where(&database.Certificate{Name: name, Enabled: true}).
		First(&model).Error
	if err != nil {
		return Certificate{}, err
	}
	certificate, err := certificateFromModel(model)
	if err != nil {
		return Certificate{}, err
	}
	version, err := r.CurrentVersion(ctx, name)
	if err != nil && !NotFound(err) {
		return Certificate{}, err
	}
	certificate.CurrentVersion = version
	return certificate, nil
}

func (r *CertificateRepository) CurrentVersion(ctx context.Context, name string) (*Version, error) {
	var model database.CertificateVersion
	err := r.database.ORM().WithContext(ctx).
		Where(&database.CertificateVersion{CertificateName: name}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true}).
		First(&model).Error
	if err != nil {
		return nil, err
	}
	version, err := versionFromModel(model)
	return &version, err
}

func (r *CertificateRepository) Versions(ctx context.Context, name string) ([]Version, error) {
	var models []database.CertificateVersion
	err := r.database.ORM().WithContext(ctx).
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

func (r *CertificateRepository) AddVersion(ctx context.Context, version Version) error {
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
	return r.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
