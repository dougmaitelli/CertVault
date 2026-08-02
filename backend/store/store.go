package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/certvault/certvault/config"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct{ db *sql.DB }
type Certificate struct {
	Name               string   `json:"name"`
	Domains            []string `json:"domains"`
	KeyType            string   `json:"key_type"`
	Status             string   `json:"status"`
	RenewBeforeSeconds int64    `json:"renew_before_seconds"`
	CurrentVersion     *Version `json:"current_version,omitempty"`
	LastError          string   `json:"last_error,omitempty"`
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
	KeyID                int64
	Name                 string
	Scopes, Certificates []string
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

const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS certificates (
    name                 TEXT PRIMARY KEY,
    domains              TEXT NOT NULL,
    key_type             TEXT NOT NULL,
    renew_before_seconds INTEGER NOT NULL,
    enabled              INTEGER NOT NULL DEFAULT 1,
    status               TEXT NOT NULL DEFAULT 'pending',
    last_error           TEXT NOT NULL DEFAULT '',
    updated_at           DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS certificate_versions (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    certificate_name   TEXT NOT NULL REFERENCES certificates(name),
    path               TEXT NOT NULL,
    domains            TEXT NOT NULL,
    serial             TEXT NOT NULL,
    issuer             TEXT NOT NULL,
    fingerprint_sha256 TEXT NOT NULL,
    not_before         DATETIME NOT NULL,
    not_after          DATETIME NOT NULL,
    created_at         DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_versions_cert
    ON certificate_versions(certificate_name, created_at DESC);

CREATE TABLE IF NOT EXISTS jobs (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    certificate_name TEXT NOT NULL,
    kind             TEXT NOT NULL,
    status           TEXT NOT NULL,
    error            TEXT NOT NULL DEFAULT '',
    started_at       DATETIME NOT NULL,
    finished_at      DATETIME
);

CREATE TABLE IF NOT EXISTS api_keys (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL UNIQUE,
    secret_hash  TEXT NOT NULL,
    scopes       TEXT NOT NULL,
    certificates TEXT NOT NULL,
    created_at   DATETIME NOT NULL,
    last_used_at DATETIME,
    last_used_ip TEXT NOT NULL DEFAULT '',
    expires_at   DATETIME,
    revoked      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS audit_events (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    at       DATETIME NOT NULL,
    actor    TEXT NOT NULL,
    action   TEXT NOT NULL,
    resource TEXT NOT NULL,
    detail   TEXT NOT NULL DEFAULT '',
    ip       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(context.Background(), schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Reconcile(ctx context.Context, c *config.Config) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, v := range c.Certificates {
		enabled := 1
		if v.Enabled != nil && !*v.Enabled {
			enabled = 0
		}
		rb := v.RenewBefore.Duration
		if rb == 0 {
			rb = 30 * 24 * time.Hour
		}
		domains, _ := json.Marshal(v.Domains)
		key := v.KeyType
		if key == "" {
			key = "ec256"
		}
		const upsertCertificate = `
			INSERT INTO certificates (
				name, domains, key_type, renew_before_seconds, enabled, updated_at
			) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET
				domains = excluded.domains,
				key_type = excluded.key_type,
				renew_before_seconds = excluded.renew_before_seconds,
				enabled = excluded.enabled,
				updated_at = excluded.updated_at`
		_, err = tx.ExecContext(
			ctx,
			upsertCertificate,
			v.Name,
			string(domains),
			key,
			int64(rb.Seconds()),
			enabled,
			time.Now().UTC(),
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListCertificates(ctx context.Context) ([]Certificate, error) {
	const query = `
		SELECT name, domains, key_type, renew_before_seconds, status, last_error
		FROM certificates
		WHERE enabled = 1
		ORDER BY name`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	out := []Certificate{}
	for rows.Next() {
		var v Certificate
		var d string
		if err := rows.Scan(&v.Name, &d, &v.KeyType, &v.RenewBeforeSeconds, &v.Status, &v.LastError); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(d), &v.Domains)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].CurrentVersion, _ = s.CurrentVersion(ctx, out[i].Name)
	}
	return out, nil
}

func (s *Store) Certificate(ctx context.Context, name string) (Certificate, error) {
	var v Certificate
	var d string
	const query = `
		SELECT name, domains, key_type, renew_before_seconds, status, last_error
		FROM certificates
		WHERE name = ? AND enabled = 1`
	err := s.db.QueryRowContext(ctx, query, name).Scan(
		&v.Name,
		&d,
		&v.KeyType,
		&v.RenewBeforeSeconds,
		&v.Status,
		&v.LastError,
	)
	_ = json.Unmarshal([]byte(d), &v.Domains)
	if err == nil {
		v.CurrentVersion, _ = s.CurrentVersion(ctx, name)
	}
	return v, err
}

func scanVersion(row interface{ Scan(...any) error }) (*Version, error) {
	var v Version
	var d string
	err := row.Scan(
		&v.ID,
		&v.CertificateName,
		&v.Path,
		&d,
		&v.Serial,
		&v.Issuer,
		&v.FingerprintSHA256,
		&v.NotBefore,
		&v.NotAfter,
		&v.CreatedAt,
	)
	_ = json.Unmarshal([]byte(d), &v.Domains)
	return &v, err
}

const selectVersions = `
	SELECT
		id, certificate_name, path, domains, serial, issuer,
		fingerprint_sha256, not_before, not_after, created_at
	FROM certificate_versions`

func (s *Store) CurrentVersion(ctx context.Context, name string) (*Version, error) {
	query := selectVersions + `
		WHERE certificate_name = ?
		ORDER BY created_at DESC
		LIMIT 1`
	return scanVersion(s.db.QueryRowContext(ctx, query, name))
}

func (s *Store) Versions(ctx context.Context, name string) ([]Version, error) {
	query := selectVersions + `
		WHERE certificate_name = ?
		ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (s *Store) AddVersion(ctx context.Context, v Version) error {
	d, _ := json.Marshal(v.Domains)
	const insertVersion = `
		INSERT INTO certificate_versions (
			certificate_name, path, domains, serial, issuer,
			fingerprint_sha256, not_before, not_after, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(
		ctx,
		insertVersion,
		v.CertificateName,
		v.Path,
		string(d),
		v.Serial,
		v.Issuer,
		v.FingerprintSHA256,
		v.NotBefore,
		v.NotAfter,
		v.CreatedAt,
	)
	if err == nil {
		const markValid = `
			UPDATE certificates
			SET status = 'valid', last_error = '', updated_at = ?
			WHERE name = ?`
		_, err = s.db.ExecContext(ctx, markValid, time.Now().UTC(), v.CertificateName)
	}
	return err
}

func (s *Store) StartJob(ctx context.Context, name, kind string) (int64, error) {
	const query = `
		INSERT INTO jobs (certificate_name, kind, status, started_at)
		VALUES (?, ?, 'running', ?)`
	result, err := s.db.ExecContext(ctx, query, name, kind, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) FinishJob(ctx context.Context, id int64, jobErr error) error {
	status := "succeeded"
	message := ""
	if jobErr != nil {
		status = "failed"
		message = jobErr.Error()
	}
	const finishJob = `
		UPDATE jobs
		SET status = ?, error = ?, finished_at = ?
		WHERE id = ?`
	_, err := s.db.ExecContext(ctx, finishJob, status, message, time.Now().UTC(), id)
	if jobErr != nil {
		const markCertificateFailed = `
			UPDATE certificates
			SET status = 'error', last_error = ?, updated_at = ?
			WHERE name = (
				SELECT certificate_name FROM jobs WHERE id = ?
			)`
		_, _ = s.db.ExecContext(
			ctx,
			markCertificateFailed,
			message,
			time.Now().UTC(),
			id,
		)
	}
	return err
}

func (s *Store) Jobs(ctx context.Context, limit int) ([]Job, error) {
	const query = `
		SELECT id, certificate_name, kind, status, error, started_at, finished_at
		FROM jobs
		ORDER BY id DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Job{}
	for rows.Next() {
		var j Job
		if err = rows.Scan(
			&j.ID,
			&j.CertificateName,
			&j.Kind,
			&j.Status,
			&j.Error,
			&j.StartedAt,
			&j.FinishedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) CreateAPIKey(ctx context.Context, name string, scopes, certs []string, expires *time.Time) (APIKey, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return APIKey{}, "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	prefixRaw := make([]byte, 5)
	if _, err := rand.Read(prefixRaw); err != nil {
		return APIKey{}, "", err
	}
	prefix := "cv_live_" + hex.EncodeToString(prefixRaw)
	token := prefix + "." + secret
	hash := sha256.Sum256([]byte(token))
	encodedScopes, _ := json.Marshal(scopes)
	encodedCertificates, _ := json.Marshal(certs)
	now := time.Now().UTC()
	const insertAPIKey = `
		INSERT INTO api_keys (
			name, prefix, secret_hash, scopes, certificates, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`
	result, err := s.db.ExecContext(
		ctx,
		insertAPIKey,
		name,
		prefix,
		hex.EncodeToString(hash[:]),
		string(encodedScopes),
		string(encodedCertificates),
		now,
		expires,
	)
	if err != nil {
		return APIKey{}, "", err
	}
	id, _ := result.LastInsertId()
	key := APIKey{
		ID:           id,
		Name:         name,
		Prefix:       prefix,
		Scopes:       scopes,
		Certificates: certs,
		CreatedAt:    now,
		ExpiresAt:    expires,
	}
	return key, token, nil
}

func (s *Store) Authenticate(ctx context.Context, token, ip string) (Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Principal{}, errors.New("invalid API key")
	}
	hash := sha256.Sum256([]byte(token))
	var id int64
	var name, storedHash, encodedScopes, encodedCertificates string
	var expires *time.Time
	var revoked bool
	const query = `
		SELECT id, name, secret_hash, scopes, certificates, expires_at, revoked
		FROM api_keys
		WHERE prefix = ?`
	err := s.db.QueryRowContext(ctx, query, parts[0]).Scan(
		&id,
		&name,
		&storedHash,
		&encodedScopes,
		&encodedCertificates,
		&expires,
		&revoked,
	)
	if err != nil || storedHash != hex.EncodeToString(hash[:]) || revoked || (expires != nil && expires.Before(time.Now())) {
		return Principal{}, errors.New("invalid API key")
	}
	var scopes, certs []string
	_ = json.Unmarshal([]byte(encodedScopes), &scopes)
	_ = json.Unmarshal([]byte(encodedCertificates), &certs)
	const recordUse = `
		UPDATE api_keys
		SET last_used_at = ?, last_used_ip = ?
		WHERE id = ?`
	_, _ = s.db.ExecContext(ctx, recordUse, time.Now().UTC(), ip, id)
	return Principal{
		KeyID:        id,
		Name:         name,
		Scopes:       scopes,
		Certificates: certs,
	}, nil
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	const query = `
		SELECT
			id, name, prefix, scopes, certificates,
			created_at, last_used_at, last_used_ip, expires_at, revoked
		FROM api_keys
		ORDER BY id DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		var encodedScopes, encodedCertificates string
		if err = rows.Scan(
			&k.ID,
			&k.Name,
			&k.Prefix,
			&encodedScopes,
			&encodedCertificates,
			&k.CreatedAt,
			&k.LastUsedAt,
			&k.LastUsedIP,
			&k.ExpiresAt,
			&k.Revoked,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(encodedScopes), &k.Scopes)
		_ = json.Unmarshal([]byte(encodedCertificates), &k.Certificates)
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIKey(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE api_keys SET revoked = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Audit(ctx context.Context, actor, action, resource, detail, ip string) {
	const query = `
		INSERT INTO audit_events (at, actor, action, resource, detail, ip)
		VALUES (?, ?, ?, ?, ?, ?)`
	_, _ = s.db.ExecContext(
		ctx,
		query,
		time.Now().UTC(),
		actor,
		action,
		resource,
		detail,
		ip,
	)
}

func (s *Store) Audits(ctx context.Context, limit int) ([]Audit, error) {
	const query = `
		SELECT id, at, actor, action, resource, detail, ip
		FROM audit_events
		ORDER BY id DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Audit
	for rows.Next() {
		var a Audit
		if err = rows.Scan(
			&a.ID,
			&a.At,
			&a.Actor,
			&a.Action,
			&a.Resource,
			&a.Detail,
			&a.IP,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (p Principal) Allows(scope, cert string) bool {
	hasScope := false
	for _, s := range p.Scopes {
		if s == scope || s == "*" {
			hasScope = true
		}
	}
	if !hasScope {
		return false
	}
	if cert == "" {
		return true
	}
	for _, c := range p.Certificates {
		if c == cert || c == "*" {
			return true
		}
	}
	return false
}

func NotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }
