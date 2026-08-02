package service

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
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

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/store"
	"github.com/certvault/certvault/vault"
	"github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/certcrypto"
	"github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/lego"
	"github.com/go-acme/lego/v5/providers/dns"
	"github.com/go-acme/lego/v5/registration"
)

type Manager struct {
	cfg     *config.Config
	db      *store.Store
	log     *slog.Logger
	locks   sync.Map
	issueMu sync.Mutex
}
type acmeUser struct {
	Email        string
	Registration *acme.ExtendedAccount
	Key          *ecdsa.PrivateKey
}

func (u *acmeUser) GetEmail() string                       { return u.Email }
func (u *acmeUser) GetRegistration() *acme.ExtendedAccount { return u.Registration }
func (u *acmeUser) GetPrivateKey() crypto.Signer           { return u.Key }

func NewManager(c *config.Config, db *store.Store, log *slog.Logger) (*Manager, error) {
	return &Manager{cfg: c, db: db, log: log}, nil
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
	certs, e := m.db.ListCertificates(ctx)
	if e != nil {
		m.log.Error("list certificates", "error", e)
		return
	}
	for _, c := range certs {
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
	lockAny, _ := m.locks.LoadOrStore(name, &sync.Mutex{})
	lock, ok := lockAny.(*sync.Mutex)
	if !ok {
		return errors.New("invalid certificate lock")
	}
	lock.Lock()
	defer lock.Unlock()
	def, ok := m.cfg.Certificate(name)
	if !ok {
		return errors.New("unknown certificate")
	}
	job, e := m.db.StartJob(ctx, name, kind)
	if e != nil {
		return e
	}
	var result error
	defer func() { _ = m.db.FinishJob(context.Background(), job, result) }()
	m.issueMu.Lock()
	defer m.issueMu.Unlock() // provider constructors consume process environment
	client, e := m.client(ctx)
	if e != nil {
		result = e
		return e
	}
	provider, e := m.provider(def)
	if e != nil {
		result = e
		return e
	}
	if e = client.Challenge.SetDNS01Provider(provider); e != nil {
		result = e
		return e
	}
	request := certificate.ObtainRequest{
		Domains: def.Domains,
		Bundle:  true,
		KeyType: certificateKeyType(def.KeyType),
	}
	resource, e := client.Certificate.Obtain(ctx, request)
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
	if e = m.db.AddVersion(ctx, v); e != nil {
		result = e
		return e
	}
	m.db.Audit(ctx, "system", "certificate."+kind, name, "", "")
	m.fireHooks(context.Background(), "certificate.issued", name, &v, nil)
	if kind != "initial" {
		m.fireHooks(context.Background(), "certificate.renewed", name, &v, nil)
	}
	return nil
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

func certificateKeyType(keyType string) certcrypto.KeyType {
	switch strings.ToLower(keyType) {
	case "ec384":
		return certcrypto.EC384
	case "rsa2048":
		return certcrypto.RSA2048
	case "rsa3072":
		return certcrypto.RSA3072
	case "rsa4096":
		return certcrypto.RSA4096
	default:
		return certcrypto.EC256
	}
}

func (m *Manager) loadUser() (*acmeUser, error) {
	dir := filepath.Join(m.cfg.DataDir, "accounts")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	path := filepath.Join(dir, "account.json.enc")
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		return &acmeUser{Email: m.cfg.ACME.Email, Key: key}, e
	}
	if e != nil {
		return nil, e
	}
	plain, e := vault.Decrypt(m.cfg.MasterKey, b)
	if e != nil {
		return nil, e
	}
	var wire struct {
		Email        string
		Key          []byte
		Registration *acme.ExtendedAccount
	}
	if e = json.Unmarshal(plain, &wire); e != nil {
		return nil, e
	}
	key, e := x509.ParseECPrivateKey(wire.Key)
	return &acmeUser{wire.Email, wire.Registration, key}, e
}

func (m *Manager) saveUser(u *acmeUser) error {
	key, e := x509.MarshalECPrivateKey(u.Key)
	if e != nil {
		return e
	}
	plain, _ := json.Marshal(struct {
		Email        string
		Key          []byte
		Registration *acme.ExtendedAccount
	}{u.Email, key, u.Registration})
	b, e := vault.Encrypt(m.cfg.MasterKey, plain)
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(m.cfg.DataDir, "accounts", "account.json.enc"), b, 0600)
}

type muxProvider struct {
	cfg       *config.Config
	cert      config.Certificate
	providers map[string]challenge.Provider
}

func (m *muxProvider) forDomain(domain string) (challenge.Provider, error) {
	name, cred, ok := m.cfg.CredentialForDomain(m.cert, domain)
	if !ok {
		return nil, fmt.Errorf("no DNS credential for %s", domain)
	}
	if p := m.providers[name]; p != nil {
		return p, nil
	}
	restore := map[string]*string{}
	for k, v := range cred.Environment {
		old, exists := os.LookupEnv(k)
		if exists {
			copy := old
			restore[k] = &copy
		} else {
			restore[k] = nil
		}
		if strings.HasSuffix(k, "_FILE") {
			b, e := os.ReadFile(v)
			if e != nil {
				return nil, e
			}
			if err := os.Setenv(strings.TrimSuffix(k, "_FILE"), strings.TrimSpace(string(b))); err != nil {
				return nil, err
			}
		} else {
			if err := os.Setenv(k, v); err != nil {
				return nil, err
			}
		}
	}
	p, e := dns.NewDNSChallengeProviderByName(cred.Provider)
	for k, v := range restore {
		if v == nil {
			if err := os.Unsetenv(k); err != nil {
				return nil, err
			}
		} else {
			if err := os.Setenv(k, *v); err != nil {
				return nil, err
			}
		}
	}
	if e != nil {
		return nil, e
	}
	m.providers[name] = p
	return p, nil
}

func (m *muxProvider) Present(ctx context.Context, domain, token, keyAuth string) error {
	p, e := m.forDomain(domain)
	if e != nil {
		return e
	}
	return p.Present(ctx, domain, token, keyAuth)
}

func (m *muxProvider) CleanUp(ctx context.Context, domain, token, keyAuth string) error {
	p, e := m.forDomain(domain)
	if e != nil {
		return e
	}
	return p.CleanUp(ctx, domain, token, keyAuth)
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

func (m *Manager) save(name string, r *certificate.Resource) (store.Version, error) {
	block, _ := pem.Decode(r.Certificate)
	if block == nil {
		return store.Version{}, errors.New("ACME response contained no certificate")
	}
	cert, e := x509.ParseCertificate(block.Bytes)
	if e != nil {
		return store.Version{}, e
	}
	now := time.Now().UTC()
	version := now.Format("20060102T150405Z")
	rel := filepath.Join("certificates", name, "versions", version)
	dir := filepath.Join(m.cfg.DataDir, rel)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return store.Version{}, e
	}
	key, e := vault.Encrypt(m.cfg.MasterKey, r.PrivateKey)
	if e != nil {
		return store.Version{}, e
	}
	fullChain := append(append([]byte{}, r.Certificate...), r.IssuerCertificate...)
	files := map[string][]byte{
		"certificate.pem":     r.Certificate,
		"chain.pem":           r.IssuerCertificate,
		"fullchain.pem":       fullChain,
		"private-key.pem.enc": key,
	}
	for n, b := range files {
		if e = atomicWrite(filepath.Join(dir, n), b, 0600); e != nil {
			return store.Version{}, e
		}
	}
	sum := sha256.Sum256(cert.Raw)
	v := store.Version{
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
		return store.Version{}, e
	}
	return v, nil
}

func (m *Manager) ReadFile(v *store.Version, name string) ([]byte, error) {
	allowed := map[string]bool{"certificate.pem": true, "chain.pem": true, "fullchain.pem": true, "private-key.pem": true}
	if !allowed[name] {
		return nil, errors.New("invalid file")
	}
	disk := name
	if name == "private-key.pem" {
		disk += ".enc"
	}
	b, e := os.ReadFile(filepath.Join(m.cfg.DataDir, v.Path, disk))
	if e != nil {
		return nil, e
	}
	if name == "private-key.pem" {
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

func (m *Manager) fireHooks(ctx context.Context, event, name string, v *store.Version, eventErr error) {
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
		if !contains(h.Events, event) {
			continue
		}
		go m.runHook(ctx, h, body)
	}
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
		cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "CERTVAULT_EVENT_JSON=" + string(body)}
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
