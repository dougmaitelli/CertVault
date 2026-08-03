package store

import (
	"time"

	"github.com/certvault/certvault/config"
)

type Certificate struct {
	Name               string         `json:"name"`
	Domains            []string       `json:"domains"`
	KeyType            config.KeyType `json:"key_type"`
	Status             string         `json:"status"`
	RenewBeforeSeconds int64          `json:"renew_before_seconds"`
	CurrentVersion     *Version       `json:"current_version,omitempty"`
	LastError          string         `json:"last_error,omitempty"`
}

type Version struct {
	ID                int64     `json:"id"`
	CertificateName   string    `json:"certificate_name"`
	Path              string    `json:"-"`
	Serial            string    `json:"serial"`
	Issuer            string    `json:"issuer"`
	FingerprintSHA256 string    `json:"fingerprint_sha256"`
	NotBefore         time.Time `json:"not_before"`
	NotAfter          time.Time `json:"not_after"`
	CreatedAt         time.Time `json:"created_at"`
	Domains           []string  `json:"domains"`
}

type Job struct {
	ID              int64      `json:"id"`
	CertificateName string     `json:"certificate_name"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	Error           string     `json:"error,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type APIKey struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Prefix       string     `json:"prefix"`
	Scopes       []string   `json:"scopes"`
	Certificates []string   `json:"certificates"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP   string     `json:"last_used_ip,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Revoked      bool       `json:"revoked"`
}

type Principal struct {
	KeyID        int64
	Name         string
	Scopes       []string
	Certificates []string
}

type Audit struct {
	ID       int64     `json:"id"`
	At       time.Time `json:"at"`
	Actor    string    `json:"actor"`
	Action   string    `json:"action"`
	Resource string    `json:"resource"`
	Detail   string    `json:"detail,omitempty"`
	IP       string    `json:"ip,omitempty"`
}
