# Client access

API keys let clients fetch certificate material without administrator access.

## Scopes

| Scope | Allows |
|---|---|
| `certificates:read` | Certificate metadata, versions, `certificate.pem`, `chain.pem`, and `fullchain.pem` |
| `private_keys:read` | `private-key.pem` downloads |
| `renewals:trigger` | Manual renewal requests |

Every key also has a certificate allowlist. Selecting “Any certificate” includes certificates added later.

## Headless API-key management

The CertVault binary can manage API keys directly in the configured database. Run it inside the application container; container access already grants access to CertVault's database and secrets, so these commands do not use HTTP authentication.

Create a key and capture the raw value shown once:

```shell
API_KEY="$(docker compose exec -T certvault certvault api-key create \
  --name traefik \
  --scope certificates:read \
  --scope private_keys:read \
  --certificate homelab)"
```

Repeat `--scope` and `--certificate` to grant multiple values. Use `--certificate '*'` to include every certificate, including certificates added later. An optional `--expires-at` accepts an RFC 3339 timestamp.

The companion commands list, revoke, and delete keys:

```shell
docker compose exec certvault certvault api-key list
docker compose exec certvault certvault api-key revoke --id 1
docker compose exec certvault certvault api-key delete --id 1
```

`list` emits JSON for automation. A key must be revoked before it can be deleted. Every mutation is recorded in the audit log with the `local-cli` actor.

## Download artifacts

```shell
curl --fail --silent --show-error \
  -H 'Authorization: Bearer cv_live_PREFIX.SECRET' \
  https://certvault.example/api/v1/certificates/homelab/fullchain.pem \
  --output fullchain.pem
```

Private keys require `private_keys:read`:

```shell
curl --fail --silent --show-error \
  -H 'Authorization: Bearer cv_live_PREFIX.SECRET' \
  https://certvault.example/api/v1/certificates/homelab/private-key.pem \
  --output private-key.pem
```

Downloads include an `ETag`, which clients may send back through `If-None-Match`. CertVault returns `304 Not Modified` when the artifact is unchanged and does not create a download audit event for that response.

## Automatic download jobs

After creating a key, the console generates an installer command for Linux and Unix-like clients. The installer:

- Stores the API key in a mode `0600` file
- Downloads into an atomic temporary directory
- Installs the selected cron schedule
- Performs the first download immediately
- Tracks an `ETag` for each file
- Uses conditional requests and avoids rewriting unchanged destination files

Running the command again replaces the existing job for that certificate and destination.
