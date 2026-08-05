# Getting started

This guide runs CertVault with Docker Compose and Let's Encrypt staging.

## 1. Create the configuration

Create `config.yaml`:

```yaml
server:
  public_url: http://localhost:8080
  log_level: info

audit:
  retention: 90d

acme:
  email: admin@example.com
  directory_url: https://acme-staging-v02.api.letsencrypt.org/directory
  accept_terms: true

dns_credentials:
  cloudflare:
    provider: cloudflare

zones:
  - name: example.com
    credential: cloudflare

certificates:
  - name: example-wildcard
    domains: [example.com, "*.example.com"]
    renew_before: 30d
```

Cloudflare is only an example. Select any provider supported by the [lego DNS provider catalog](https://go-acme.github.io/lego/dns/).

## 2. Create secrets

Create an untracked `.env` file:

```dotenv
CERTVAULT_MASTER_KEY=replace-with-output-of-openssl-rand-base64-32
CERTVAULT_BOOTSTRAP_ADMIN_TOKEN=replace-with-a-long-random-login-token
CLOUDFLARE_DNS_API_TOKEN=replace-with-a-zone-restricted-token
```

This guide uses the optional bootstrap token as the simplest initial login method. You may omit it when configuring OIDC instead.

Generate the master key with:

```shell
openssl rand -base64 32
```

Back up the master key separately and securely. Stored certificate and ACME account keys cannot be recovered without it.

## 3. Start CertVault

Create `docker-compose.yaml`:

```yaml
services:
  certvault:
    image: ghcr.io/dougmaitelli/certvault:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    env_file: [.env]
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - certvault-data:/data
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL

volumes:
  certvault-data:
```

Start the service:

```shell
docker compose up -d
```

Open `http://localhost:8080` and sign in with the bootstrap administrator token.

## 4. Move to production

After staging issuance succeeds:

1. Put CertVault behind an HTTPS reverse proxy.
2. Change the ACME directory to `https://acme-v02.api.letsencrypt.org/directory`.
3. Back up the data volume and master key together.
4. Create scoped API keys for certificate consumers.

Production and staging ACME accounts are separate registrations.
