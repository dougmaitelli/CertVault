package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/certvault/certvault/audit"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database/repository"
	"github.com/certvault/certvault/vault"
	"github.com/go-acme/lego/v5/certcrypto"
	"github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/go-acme/lego/v5/lego"
	"github.com/go-acme/lego/v5/registration"
)

type Manager struct {
	cfg     *config.Config
	repos   *repository.Repositories
	log     *slog.Logger
	locks   sync.Map
	issueMu sync.Mutex
}

func NewManager(c *config.Config, repos *repository.Repositories, log *slog.Logger) (*Manager, error) {
	return &Manager{cfg: c, repos: repos, log: log}, nil
}

func (m *Manager) Run(ctx context.Context) {
	m.reconcile(ctx)

	tick := time.NewTicker(6 * time.Hour)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.reconcile(ctx)
		}
	}
}

func (m *Manager) reconcile(ctx context.Context) {
	certs, e := m.repos.Certificates.List(ctx)
	if e != nil {
		m.log.Error("list certificates", "error", e)
		return
	}

	for _, c := range certs {
		definition, ok := m.cfg.Certificate(c.Name)
		if !ok || !m.cfg.ShouldAutomaticallyIssue(definition) {
			continue
		}

		due := c.CurrentVersion == nil || time.Until(c.CurrentVersion.NotAfter) < time.Duration(c.RenewBeforeSeconds)*time.Second
		if due {
			go func(name string) {
				if e := m.Issue(context.Background(), name, "scheduled"); e != nil {
					m.log.Error("certificate issuance failed", "certificate", name, "error", e)
				}
			}(c.Name)
		}
	}
}

func (m *Manager) Issue(ctx context.Context, name, kind string) error {
	var auditAction string

	switch kind {
	case "initial":
		auditAction = audit.ActionCertificateInitial
	case "manual":
		auditAction = audit.ActionCertificateManual
	case "scheduled":
		auditAction = audit.ActionCertificateScheduled
	default:
		return fmt.Errorf("unsupported issuance kind %q", kind)
	}

	def, ok := m.cfg.Certificate(name)
	if !ok {
		return errors.New("unknown certificate")
	}

	lockAny, _ := m.locks.LoadOrStore(name, &sync.Mutex{})

	lock, ok := lockAny.(*sync.Mutex)
	if !ok {
		return errors.New("invalid certificate lock")
	}

	lock.Lock()
	defer lock.Unlock()

	job, e := m.repos.Jobs.Start(ctx, name, kind)
	if e != nil {
		return e
	}

	var result error
	defer func() { _ = m.repos.Jobs.Finish(context.Background(), job, result) }()

	resource, e := m.obtain(ctx, def)
	if e != nil {
		result = e
		m.fireHooks(context.Background(), "certificate.failed", name, nil, e)

		return e
	}

	v, e := m.save(name, resource)
	if e != nil {
		result = e
		return e
	}

	if e = m.repos.Certificates.AddVersion(ctx, v); e != nil {
		result = e
		return e
	}

	m.repos.Audits.Record(
		ctx, audit.ActorSystem, auditAction, name, "", "",
	)
	m.fireHooks(context.Background(), "certificate.issued", name, &v, nil)

	if kind != "initial" {
		m.fireHooks(context.Background(), "certificate.renewed", name, &v, nil)
	}

	return nil
}

func (m *Manager) obtain(ctx context.Context, def config.Certificate) (*certificate.Resource, error) {
	if m.cfg.ACME.Mock {
		return mockCertificate(def)
	}

	m.issueMu.Lock()
	defer m.issueMu.Unlock() // provider constructors consume process environment

	client, e := m.client(ctx)
	if e != nil {
		return nil, e
	}

	provider, e := m.provider(def)
	if e != nil {
		return nil, e
	}

	dns01.SetDefaultClient(dns01.NewClient(&dns01.Options{
		RecursiveNameservers: m.cfg.ACME.DNSResolvers,
	}))

	if e = client.Challenge.SetDNS01Provider(provider); e != nil {
		return nil, e
	}

	keyType, e := certcrypto.ToKeyType(string(def.KeyType))
	if e != nil {
		return nil, fmt.Errorf("certificate %q key type: %w", def.Name, e)
	}

	request := certificate.ObtainRequest{
		Domains: def.Domains,
		Bundle:  true,
		KeyType: keyType,
	}

	resource, e := client.Certificate.Obtain(ctx, request)
	if e != nil {
		return nil, e
	}

	return resource, nil
}

func (m *Manager) client(ctx context.Context) (*lego.Client, error) {
	user, e := m.loadUser()
	if e != nil {
		return nil, e
	}

	cfg := lego.NewConfig(user)
	cfg.CADirURL = m.cfg.ACME.DirectoryURL

	client, e := lego.NewClient(cfg)
	if e != nil {
		return nil, e
	}

	if user.Registration == nil {
		reg, e := client.Registration.Register(ctx, registration.RegisterOptions{TermsOfServiceAgreed: m.cfg.ACME.AcceptTerms})
		if e != nil {
			return nil, e
		}

		user.Registration = reg
		if e = m.saveUser(user); e != nil {
			return nil, e
		}
	}

	return client, nil
}

func (m *Manager) provider(c config.Certificate) (challenge.Provider, error) {
	p := &muxProvider{cfg: m.cfg, cert: c, providers: map[string]challenge.Provider{}}
	for _, d := range c.Domains {
		if _, e := p.forDomain(strings.TrimPrefix(d, "*.")); e != nil {
			return nil, e
		}
	}

	return p, nil
}

func (m *Manager) save(name string, r *certificate.Resource) (repository.Version, error) {
	block, _ := pem.Decode(r.Certificate)
	if block == nil {
		return repository.Version{}, errors.New("ACME response contained no certificate")
	}

	cert, e := x509.ParseCertificate(block.Bytes)
	if e != nil {
		return repository.Version{}, e
	}

	now := time.Now().UTC()
	version := now.Format("20060102T150405.000000000Z")
	rel := filepath.Join("certificates", name, "versions", version)

	dir := filepath.Join(m.cfg.DataDir, rel)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return repository.Version{}, e
	}

	key, e := vault.Encrypt(m.cfg.MasterKey, r.PrivateKey)
	if e != nil {
		return repository.Version{}, e
	}

	fullChain := append(append([]byte{}, r.Certificate...), r.IssuerCertificate...)

	files := map[string][]byte{
		"certificate.crt": r.Certificate,
		"chain.crt":       r.IssuerCertificate,
		"fullchain.crt":   fullChain,
		"private.key.enc": key,
	}
	for n, b := range files {
		if e = atomicWrite(filepath.Join(dir, n), b, 0600); e != nil {
			return repository.Version{}, e
		}
	}

	sum := sha256.Sum256(cert.Raw)
	v := repository.Version{
		CertificateName:   name,
		Path:              rel,
		Serial:            cert.SerialNumber.String(),
		Issuer:            cert.Issuer.String(),
		FingerprintSHA256: hex.EncodeToString(sum[:]),
		NotBefore:         cert.NotBefore,
		NotAfter:          cert.NotAfter,
		CreatedAt:         now,
		Domains:           cert.DNSNames,
	}

	meta, _ := json.MarshalIndent(v, "", "  ")
	if e = atomicWrite(filepath.Join(dir, "metadata.json"), meta, 0600); e != nil {
		return repository.Version{}, e
	}

	return v, nil
}

func (m *Manager) ReadFile(v *repository.Version, name string) ([]byte, error) {
	allowed := map[string]bool{"certificate.crt": true, "chain.crt": true, "fullchain.crt": true, "private.key": true}
	if !allowed[name] {
		return nil, errors.New("invalid file")
	}

	disk := name
	if name == "private.key" {
		disk += ".enc"
	}

	b, e := os.ReadFile(filepath.Join(m.cfg.DataDir, v.Path, disk))
	if errors.Is(e, os.ErrNotExist) {
		legacy := map[string]string{
			"certificate.crt": "certificate.pem",
			"chain.crt":       "chain.pem",
			"fullchain.crt":   "fullchain.pem",
			"private.key":     "private-key.pem.enc",
		}
		b, e = os.ReadFile(filepath.Join(m.cfg.DataDir, v.Path, legacy[name]))
	}

	if e != nil {
		return nil, e
	}

	if name == "private.key" {
		return vault.Decrypt(m.cfg.MasterKey, b)
	}

	return b, nil
}

func atomicWrite(path string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}

	f, e := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if e != nil {
		return e
	}

	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}

	if e == nil {
		e = f.Sync()
	}

	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}

	if e != nil {
		return e
	}

	return os.Rename(tmp, path)
}

func (m *Manager) fireHooks(ctx context.Context, event, name string, v *repository.Version, eventErr error) {
	payload := map[string]any{
		"id":          fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		"event":       event,
		"timestamp":   time.Now().UTC(),
		"certificate": name,
		"version":     v,
	}
	if eventErr != nil {
		payload["error"] = eventErr.Error()
	}

	body, _ := json.Marshal(payload)

	for _, h := range m.cfg.Hooks {
		if !hookMatches(h, event, name) {
			continue
		}
		go m.runHook(ctx, h, body)
	}
}

func hookMatches(h config.Hook, event, certificate string) bool {
	return contains(h.Events, event) &&
		(len(h.Certificates) == 0 || contains(h.Certificates, certificate))
}

func (m *Manager) runHook(ctx context.Context, h config.Hook, body []byte) {
	timeout := h.Timeout.Duration
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var hookErr error

	switch h.Type {
	case "webhook":
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, strings.NewReader(string(body)))
		if err != nil {
			m.log.Error("create webhook request", "hook", h.Name, "error", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")

		if h.SecretFile != "" {
			secret, readErr := os.ReadFile(h.SecretFile)
			if readErr != nil {
				m.log.Error("read webhook secret", "hook", h.Name, "error", readErr)
				return
			}

			mac := hmac.New(sha256.New, []byte(strings.TrimSpace(string(secret))))
			_, _ = mac.Write(body)
			req.Header.Set("X-CertVault-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}

		resp, err := http.DefaultClient.Do(req)
		hookErr = err

		if resp != nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				hookErr = fmt.Errorf("HTTP %s", resp.Status)
			}
		}
	case "exec":
		cmd := exec.CommandContext(ctx, h.Command, h.Args...)
		cmd.Env = []string{
			config.EnvPath + "=/usr/local/bin:/usr/bin:/bin",
			config.EnvEventJSON + "=" + string(body),
		}
		_, hookErr = cmd.CombinedOutput()
	default:
		hookErr = errors.New("unknown hook type")
	}

	if hookErr != nil {
		m.log.Error("hook delivery failed", "hook", h.Name, "error", hookErr)
	}
}

func contains(v []string, w string) bool {
	for _, x := range v {
		if x == w {
			return true
		}
	}

	return false
}

func clientIP(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		return host
	}

	return r.RemoteAddr
}

var _ = clientIP
