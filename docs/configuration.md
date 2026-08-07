# Configuration

CertVault reads YAML from `/config/config.yaml` by default. The reference file includes every supported section with annotated examples.

[View the full configuration reference](https://github.com/dougmaitelli/CertVault/blob/master/config/config.example.yaml){ .md-button }
[Download the YAML file](https://raw.githubusercontent.com/dougmaitelli/CertVault/master/config/config.example.yaml){ .md-button }

## Core settings

| Setting | Purpose |
|---|---|
| `data_dir` | SQLite database and encrypted certificate storage |
| `server.listen` | HTTP listen address |
| `server.public_url` | Browser-visible origin and OIDC callback base |
| `server.log_level` | `debug`, `info`, `warn`, or `error` |
| `server.trusted_proxies` | Proxy IP addresses or CIDRs trusted to supply client IP headers |
| `audit.retention` | Age after which audit events are deleted; omitted means indefinitely |
| `auth.session_duration` | Browser session lifetime; defaults to `8h` |
| `acme.email` | ACME account contact address |
| `acme.directory_url` | ACME v2 directory |
| `acme.accept_terms` | Must be `true` |
| `acme.automatic_issuance` | Global automatic issuance switch |

Durations accept Go duration syntax and whole days, such as `12h`, `30d`, and `90d`.

## Certificates

```yaml
certificates:
  - name: homelab-wildcard
    domains:
      - example.com
      - "*.example.com"
    key_type: ec256
    renew_before: 30d
    credential: cloudflare_main
    automatic_issuance: true
    enabled: true
```

Supported key types are `ec256` (default), `ec384`, `rsa2048`, `rsa3072`, and `rsa4096`.

## Environment overrides

| Variable | Purpose |
|---|---|
| `CERTVAULT_CONFIG` | Configuration path |
| `CERTVAULT_DATA_DIR` | Persistent data directory |
| `CERTVAULT_LISTEN` | HTTP listen address |
| `CERTVAULT_PUBLIC_URL` | Browser-visible origin |
| `CERTVAULT_LOG_LEVEL` | Logging level |
| `CERTVAULT_ACME_EMAIL` | ACME account email |
| `CERTVAULT_ACME_DIRECTORY_URL` | ACME directory |
| `CERTVAULT_ACME_DNS_RESOLVERS` | Comma-separated DNS-01 resolvers |
| `CERTVAULT_MASTER_KEY` | Base64-encoded 32-byte encryption key |
| `CERTVAULT_MASTER_KEY_FILE` | File containing the master key |
| `CERTVAULT_SESSION_DURATION` | Browser session lifetime, such as `8h` or `1d` |
| `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN` | Bootstrap UI credential |
| `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE` | File containing the bootstrap credential |
| `CERTVAULT_OIDC_ISSUER_URL` | OIDC issuer |
| `CERTVAULT_OIDC_CLIENT_ID` | OIDC client ID |
| `CERTVAULT_OIDC_CLIENT_SECRET` | OIDC client secret |
| `CERTVAULT_OIDC_CLIENT_SECRET_FILE` | File containing the OIDC client secret |
| `CERTVAULT_OIDC_ALLOWED_GROUPS` | Comma-separated administrator groups |

Do not set both a direct secret and its `_FILE` counterpart.

Bootstrap authentication is optional. Omit `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN`, `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE`, and `auth.bootstrap_token_file` to disable it. Configure OIDC if administrators still need web-console access.
