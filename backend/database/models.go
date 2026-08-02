package database

import "time"

type Certificate struct {
	Name               string    `gorm:"primaryKey"`
	Domains            string    `gorm:"not null"`
	KeyType            string    `gorm:"not null"`
	RenewBeforeSeconds int64     `gorm:"not null"`
	Enabled            bool      `gorm:"not null;default:true"`
	Status             string    `gorm:"not null;default:pending"`
	LastError          string    `gorm:"not null;default:''"`
	UpdatedAt          time.Time `gorm:"not null"`
}

func (Certificate) TableName() string { return "certificates" }

type CertificateVersion struct {
	ID                int64     `gorm:"primaryKey;autoIncrement"`
	CertificateName   string    `gorm:"not null;index:idx_versions_cert,priority:1"`
	Path              string    `gorm:"not null"`
	Domains           string    `gorm:"not null"`
	Serial            string    `gorm:"not null"`
	Issuer            string    `gorm:"not null"`
	FingerprintSHA256 string    `gorm:"not null"`
	NotBefore         time.Time `gorm:"not null"`
	NotAfter          time.Time `gorm:"not null"`
	CreatedAt         time.Time `gorm:"not null;index:idx_versions_cert,priority:2,sort:desc"`
}

func (CertificateVersion) TableName() string { return "certificate_versions" }

type Job struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	CertificateName string    `gorm:"not null"`
	Kind            string    `gorm:"not null"`
	Status          string    `gorm:"not null"`
	Error           string    `gorm:"not null;default:''"`
	StartedAt       time.Time `gorm:"not null"`
	FinishedAt      *time.Time
}

func (Job) TableName() string { return "jobs" }

type APIKey struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	Name         string    `gorm:"not null"`
	Prefix       string    `gorm:"not null;uniqueIndex"`
	SecretHash   string    `gorm:"not null"`
	Scopes       string    `gorm:"not null"`
	Certificates string    `gorm:"not null"`
	CreatedAt    time.Time `gorm:"not null"`
	LastUsedAt   *time.Time
	LastUsedIP   string `gorm:"not null;default:''"`
	ExpiresAt    *time.Time
	Revoked      bool `gorm:"not null;default:false"`
}

func (APIKey) TableName() string { return "api_keys" }

type AuditEvent struct {
	ID       int64     `gorm:"primaryKey;autoIncrement"`
	At       time.Time `gorm:"not null"`
	Actor    string    `gorm:"not null"`
	Action   string    `gorm:"not null"`
	Resource string    `gorm:"not null"`
	Detail   string    `gorm:"not null;default:''"`
	IP       string    `gorm:"not null;default:''"`
}

func (AuditEvent) TableName() string { return "audit_events" }

type Setting struct {
	Key   string `gorm:"primaryKey"`
	Value string `gorm:"not null"`
}

func (Setting) TableName() string { return "settings" }
