package repository

import "github.com/certvault/certvault/database"

type Repositories struct {
	Certificates *CertificateRepository
	Jobs         *JobRepository
	APIKeys      *APIKeyRepository
	Audits       *AuditRepository
}

func New(db *database.Database) *Repositories {
	return &Repositories{
		Certificates: &CertificateRepository{database: db},
		Jobs:         &JobRepository{database: db},
		APIKeys:      &APIKeyRepository{database: db},
		Audits:       &AuditRepository{database: db},
	}
}
