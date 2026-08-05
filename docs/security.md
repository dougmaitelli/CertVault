# Security

CertVault handles private keys and DNS credentials. Treat the service as security-sensitive infrastructure.

## Storage

- Certificate and ACME account private keys are encrypted with AES-256-GCM.
- The master key is external to SQLite and must be backed up separately.
- Raw API keys and DNS credentials are not stored in SQLite.
- Certificate versions use temporary files and atomic renames.

## Network exposure

Always use HTTPS when exposing CertVault over a network. Configure `server.trusted_proxies` only with proxy addresses or CIDRs you operate. Never trust a broad untrusted network such as `0.0.0.0/0`.

CertVault walks trusted proxy chains from right to left so client-supplied forwarding entries cannot override the address added by a trusted proxy.

## Container hardening

The published container is designed to run without Linux capabilities. Recommended Compose settings include:

```yaml
security_opt:
  - no-new-privileges:true
cap_drop:
  - ALL
```

Mount configuration and secret files read-only. Back up the persistent volume and master key together.

## Vulnerability reporting

Report vulnerabilities privately through [GitHub Security Advisories](https://github.com/dougmaitelli/CertVault/security/advisories/new). Do not open a public issue containing sensitive details.
