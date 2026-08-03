# CertVault

CertVault is a self-hosted ACME certificate controller for homelabs. It obtains wildcard and multi-domain certificates through DNS-01, renews them automatically, stores private material encrypted, and exposes scoped download endpoints for other machines on the local network. The web console shows certificate health, issuance history, and API-key usage.

## Features

- Headless YAML and environment configuration
- Multiple DNS zones, credentials, domains, and certificates
- Every DNS provider supported by [`go-acme/lego`](https://go-acme.github.io/lego/dns/)
- Let's Encrypt staging/production or another ACME v2 directory
- Encrypted ACME account keys and certificate private keys (AES-256-GCM)
- Immutable, atomic certificate versions with GORM-managed SQLite metadata
- Six-hour renewal reconciliation with per-certificate locking
- Hashed, revocable API keys scoped by operation and certificate
- Optional OIDC Authorization Code + PKCE login and group allowlisting
- Signed webhooks and restricted executable hooks
- Responsive React operations console
- Non-root, capability-free Docker deployment

## Quick start

The example deliberately uses Let's Encrypt staging. Do not switch to production until staging issuance succeeds.

```sh
cp config/config.example.yaml config/config.yaml
mkdir -p secrets hooks
openssl rand -base64 32 > secrets/master-key
openssl rand -base64 32 > secrets/admin-token
printf '%s' 'your-scoped-cloudflare-token' > secrets/cloudflare-token
chmod 600 secrets/*
```

Edit `config/config.yaml`, replacing the email, public URL, zone, and certificate domains. Then run:

```sh
docker compose up --build -d
docker compose logs -f certvault
```

Open `http://localhost:8080` and sign in with the content of `secrets/admin-token`. Put CertVault behind an HTTPS reverse proxy before using it across the network—the API can return private keys and must not be exposed over plaintext HTTP.

When staging succeeds, change the directory URL to:

```text
https://acme-v02.api.letsencrypt.org/directory
```

The production and staging ACME accounts are cryptographically separate. Keep the `/data` volume and `secrets/master-key` backed up together. The encrypted data cannot be recovered without that master key.

## Cloudflare permissions

Create an API token restricted to the required zones with `Zone:Read` and `DNS:Edit`. Do not use the Cloudflare global API key. The token is read from the Docker secret and is never persisted in SQLite.

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
| `CERTVAULT_MASTER_KEY_FILE` | Base64-encoded 32-byte encryption key |
| `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE` | Break-glass UI token |
| `CERTVAULT_UI_DIR` | Built frontend directory |

DNS provider environment entries may end in `_FILE`. CertVault reads that file and supplies its contents to the provider without retaining the value.

`server.log_level` and `CERTVAULT_LOG_LEVEL` accept `debug`, `info`, `warn`
(`warning` is also accepted), `error`, and slog offsets such as `DEBUG+2`.

Certificate key types are `ec256` (default), `ec384`, `rsa2048`, `rsa3072`, and `rsa4096`. `renew_before` uses Go duration syntax; `720h` is 30 days.

## OIDC

Uncomment `auth.oidc` in the example. Register its exact `redirect_url` with the provider and mount the client secret file. If `allowed_groups` is empty, any successfully authenticated identity is an administrator; setting it is strongly recommended. The bootstrap token remains available as break-glass access.

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
- Run only one CertVault replica against a data volume.
- Back up the volume and master key separately and test restoration.

The service trusts proxy termination only for transport; it does not trust forwarded client addresses. Audit IPs therefore record the direct peer, which avoids spoofing unless explicit trusted-proxy support is added later.

## Development

```sh
make dependencies
make check
cd backend
CERTVAULT_UI_DIR="$PWD/../web/dist" go run . -config ../config/config.yaml
```

The repository pins its frontend dependencies in `package-lock.json` and installs
golangci-lint v2.12.2 into the ignored `.bin` directory. The main quality commands
are:

```sh
make format        # gofmt and Prettier
make format-check  # verify formatting without changing files
make vet           # go vet
make lint          # golangci-lint and ESLint
make test          # Go tests and frontend type checking
make build         # backend binary and frontend production bundle
make check         # run every non-mutating verification above
```

Frontend commands can also be run independently from `web/` with `npm run
format`, `npm run lint`, `npm run typecheck`, `npm run test`, `npm run build`, or
`npm run check`.

The backend's `database` package owns the SQLite connection and GORM schema
migrations. Its `database/repository` subpackage provides separate certificate,
job, API-key, and audit repositories; none owns connection setup or handwritten
SQL. HTTP authentication lives in `api/auth`.

The API outline is in [`docs/openapi.yaml`](docs/openapi.yaml).
