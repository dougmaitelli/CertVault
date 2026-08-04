<p align="center">
  <img src="assets/logo.svg" alt="CertVault — centrally managed TLS certificates" width="900">
</p>

[![CI](https://github.com/dougmaitelli/CertVault/actions/workflows/ci.yml/badge.svg)](https://github.com/dougmaitelli/CertVault/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/dougmaitelli/CertVault)](https://github.com/dougmaitelli/CertVault/releases)
[![GHCR](https://img.shields.io/badge/container-ghcr.io-blue)](https://github.com/dougmaitelli/CertVault/pkgs/container/certvault)

CertVault is a self-hosted ACME certificate controller for homelabs. It obtains wildcard and multi-domain certificates through DNS-01, renews them automatically, stores private material encrypted, and exposes scoped download endpoints for other machines on the local network. The web console shows certificate health, issuance history, and API-key usage.

## Project documentation

- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

## Screenshots

<table>
<tr>
<td width="50%">
<a href="screenshots/certificates.png"><img src="screenshots/certificates.png" alt="Certificate management dashboard"></a>
<p><em>Certificate inventory — current health, validity, domains, key types, downloads, and manual renewal controls.</em></p>
</td>
<td width="50%">
<a href="screenshots/certificate-details.png"><img src="screenshots/certificate-details.png" alt="Certificate details and version history"></a>
<p><em>Certificate inspection — identity, validity, downloadable artifacts, and an immutable version timeline.</em></p>
</td>
</tr>
<tr>
<td width="50%">
<a href="screenshots/acme-accounts.png"><img src="screenshots/acme-accounts.png" alt="ACME account management"></a>
<p><em>ACME accounts — separate registrations and status for every certificate authority directory.</em></p>
</td>
<td width="50%">
<a href="screenshots/api-keys.png"><img src="screenshots/api-keys.png" alt="Scoped API key management"></a>
<p><em>Machine access — scoped, revocable API keys with certificate allowlists and usage visibility.</em></p>
</td>
</tr>
</table>

## Features

- Headless YAML and environment configuration
- Multiple DNS zones, credentials, domains, and certificates
- Every DNS provider supported by [`go-acme/lego`](https://go-acme.github.io/lego/dns/)
- Let's Encrypt staging/production or another ACME v2 directory
- Encrypted ACME account keys and certificate private keys (AES-256-GCM)
- Immutable, atomic certificate versions with GORM-managed SQLite metadata
- Six-hour renewal reconciliation with per-certificate locking
- Hashed, revocable API keys scoped by operation and certificate
- Optional OIDC Authorization + PKCE login
- Signed webhooks and restricted executable hooks
- Responsive React Console
- Non-root, capability-free Docker deployment

## Quick start

Create `config.yaml` and replace the example email and domain. The example uses Let's Encrypt staging so initial testing cannot exhaust production rate limits.

```yaml
server:
  public_url: http://localhost:8080
  log_level: info
  # Only add the IP or network of a reverse proxy you operate.
  # trusted_proxies: [172.18.0.0/16]

acme:
  email: admin@example.com
  directory_url: https://acme-staging-v02.api.letsencrypt.org/directory
  accept_terms: true

# Uncomment to enable OIDC. Keep the client secret in .env.
# auth:
#   oidc:
#     issuer_url: https://auth.example.com
#     client_id: certvault
#     allowed_groups: [cert-admins]

dns_credentials:
  cloudflare:
    provider: cloudflare

zones:
  - name: example.com
    credential: cloudflare

certificates:
  - name: example-wildcard
    domains: [example.com, "*.example.com"]
    renew_before: 720h
```

Create an untracked `.env` file beside it. Generate the master key with `openssl rand -base64 32`, then replace all three values:

```dotenv
CERTVAULT_MASTER_KEY=replace-with-output-of-openssl-rand-base64-32
CERTVAULT_BOOTSTRAP_ADMIN_TOKEN=replace-with-a-long-random-login-token
CLOUDFLARE_DNS_API_TOKEN=replace-with-cloudflare-token
# CERTVAULT_OIDC_CLIENT_SECRET=replace-with-oidc-client-secret
```

Create `docker-compose.yaml` beside both files:

```yaml
services:
  certvault:
    image: ghcr.io/dougmaitelli/certvault:latest
    container_name: certvault
    restart: unless-stopped
    ports:
      - "8080:8080"
    env_file:
      - .env
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - certvault-data:/data
    healthcheck:
      test:
        - CMD-SHELL
        - wget -q --spider http://127.0.0.1:8080/api/v1/health || exit 1
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL

volumes:
  certvault-data:
```

```sh
docker compose up -d
```

Open `http://localhost:8080` and sign in with the value assigned to `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN`.

Put CertVault behind an HTTPS reverse proxy before using it across the network—the API can return private keys and must not be exposed over plaintext HTTP.

When staging succeeds, change the directory URL to:

```text
https://acme-v02.api.letsencrypt.org/directory
```

The production and staging ACME accounts are cryptographically separate. Back up the `/data` volume and master key together. Encrypted data cannot be recovered without that key.

## DNS providers

Cloudflare is used only as a familiar Quick Start example. It is not required. CertVault supports the DNS providers available through lego; choose a provider and its required environment variables from the [lego DNS provider documentation](https://go-acme.github.io/lego/dns/).

For the Cloudflare example, create an API token restricted to the required zones with `Zone:Read` and `DNS:Edit`. Do not use the Cloudflare global API key. DNS credentials are not persisted in SQLite.

## Machine access

Create an API key in the web console. The raw value is shown once. Fetch artifacts with:

```sh
curl --fail --silent --show-error \
  -H 'Authorization: Bearer cv_live_PREFIX.SECRET' \
  https://certvault.home.example.com/api/v1/certificates/homelab-wildcard/fullchain.pem \
  --output fullchain.pem

curl --fail --silent --show-error \
  -H 'Authorization: Bearer cv_live_PREFIX.SECRET' \
  https://certvault.home.example.com/api/v1/certificates/homelab-wildcard/private-key.pem \
  --output private-key.pem
```

Downloads include an `ETag`, so clients may use `If-None-Match`. A machine key needs `certificates:read` for public certificate files and the separately privileged `private_keys:read` scope for private keys.

The API Keys page can also generate a one-time command that installs an automatic download job through `/client/install.sh`. The installer requires a Linux or Unix-like client with `curl`, `crontab`, and `install`. It stores the API key in a mode `0600` file, downloads through an atomic temporary directory, installs the selected cron schedule, immediately performs the first download, and replaces an existing job for the same certificate files instead of creating duplicates. The default bundle installs `fullchain.pem` and `private-key.pem` together into a certificate-specific destination directory. The client stores each file's `ETag` and uses conditional requests on later runs; unchanged files return `304 Not Modified`, are not rewritten, and do not create certificate-download audit events.

## Configuration

Global values can be overridden with:

| Variable | Purpose |
|---|---|
| `CERTVAULT_CONFIG` | YAML path; defaults to `/config/config.yaml` |
| `CERTVAULT_DATA_DIR` | Persistent data directory |
| `CERTVAULT_LISTEN` | HTTP listen address |
| `CERTVAULT_PUBLIC_URL` | Browser-visible HTTPS origin |
| `CERTVAULT_ACME_EMAIL` | ACME account email |
| `CERTVAULT_ACME_DIRECTORY_URL` | ACME directory override |
| `CERTVAULT_ACME_DNS_RESOLVERS` | Comma-separated recursive resolvers used for DNS-01 checks; defaults to `1.1.1.1:53,1.0.0.1:53` |
| `CERTVAULT_MASTER_KEY` | Base64-encoded 32-byte encryption key |
| `CERTVAULT_MASTER_KEY_FILE` | Path to a file containing the Base64-encoded encryption key |
| `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN` | Break-glass UI token |
| `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE` | Path to a file containing the break-glass UI token |
| `CERTVAULT_OIDC_ISSUER_URL` | OIDC issuer used for provider discovery |
| `CERTVAULT_OIDC_CLIENT_ID` | OIDC client identifier |
| `CERTVAULT_OIDC_CLIENT_SECRET` | OIDC client secret |
| `CERTVAULT_OIDC_CLIENT_SECRET_FILE` | Path to a file containing the OIDC client secret |
| `CERTVAULT_OIDC_ALLOWED_GROUPS` | Comma-separated administrator group allowlist |
| `CERTVAULT_UI_DIR` | Built frontend directory |

Some values may end in `_FILE`. CertVault reads that file and supplies its contents to the provider without retaining the value.

Set either `CERTVAULT_MASTER_KEY` directly or `CERTVAULT_MASTER_KEY_FILE` to the path of a mounted secret. They cannot both be set. The file form is recommended for Docker because the key is not exposed in the container's environment metadata.

The bootstrap administrator credential follows the same pattern: set either `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN` directly or `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE` to a secret file path, but not both. The file form remains recommended for Docker deployments.

Certificate key types are `ec256` (default), `ec384`, `rsa2048`, `rsa3072`, and `rsa4096`. `renew_before` uses Go duration syntax; `720h` is 30 days.

Set `acme.automatic_issuance` to control initial issuance and scheduled renewal globally. A certificate can set its own `automatic_issuance` value to override the global setting; omitted values inherit it. Manual renewal requests remain available regardless of these settings.

## OIDC

Uncomment `auth.oidc` in the Quick Start and add `CERTVAULT_OIDC_CLIENT_SECRET` to `.env`. Register `{server.public_url}/auth/callback` with the provider. The client secret may instead be supplied through `CERTVAULT_OIDC_CLIENT_SECRET_FILE`, but both forms cannot be set together. If `allowed_groups` is empty, any successfully authenticated identity is an administrator; setting an allowlist is strongly recommended. The bootstrap token remains available as break-glass access.

## Hooks

Webhook bodies identify the event, certificate, version, and timestamp. If `secret_file` is configured, requests contain:

```text
X-CertVault-Signature-256: sha256=<hex HMAC-SHA256 of body>
```

Executable hooks are disabled unless explicitly present in YAML. They run directly without a shell, receive only a minimal `PATH` and `CERTVAULT_EVENT_JSON`, and are killed at the configured timeout. Mount executables read-only under `/hooks`.

Supported events are `certificate.issued`, `certificate.renewed`, and `certificate.failed`.

## Data and security model

- SQLite contains metadata, hashes, history, and audit events—not raw API keys or DNS credentials.
- Certificate and ACME private keys are authenticated-encrypted with the external master key.
- Certificate versions are written through temporary files and atomic renames.
- A failed renewal leaves the prior version untouched.
- Only one issuance runs at a time because lego provider construction reads process environment.

By default, audit entries record the direct network peer and ignore forwarded address headers. Set `server.trusted_proxies` to proxy IPs or CIDRs to accept `X-Forwarded-For` and `X-Real-IP` only from those peers. CertVault walks trusted proxy chains from right to left so client-supplied entries cannot override the address added by your proxy. Never configure an untrusted network such as `0.0.0.0/0`.

## Development

To run the UI and API locally without configuring DNS or issuing certificates:

```sh
make dev
```

Open `http://localhost:8081` and use `certvault-dev-admin` as the bootstrap token. Development data and generated secrets are stored under the ignored `.cache/dev` directory. The development configuration enables the mock ACME issuer, so the configured certificates can be issued and renewed from the UI without making ACME or DNS-provider requests. These locally signed certificates are untrusted and intended only for testing CertVault's workflows.

The main quality commands are:

```sh
make format        # gofmt and Prettier
make format-check  # verify formatting without changing files
make vet           # go vet
make lint          # golangci-lint and ESLint
make test          # Go tests and frontend type checking
make build         # backend binary and frontend production bundle
make check         # run every non-mutating verification above
```

Frontend commands can also be run independently from `web/` with `npm run format`, `npm run lint`, `npm run typecheck`, `npm run test`, `npm run build`, or `npm run check`.

Regenerate the README screenshots from deterministic mocked API data with `make screenshots`. This requires Playwright's Chromium browser (`cd web && npx playwright install chromium`). To use Playwright's prebuilt browser container instead, run `make screenshots-docker` with Docker available.

The API outline is in [`docs/openapi.yaml`](docs/openapi.yaml).
