---
title: Client access
description: Create scoped API keys and securely retrieve certificate artifacts.
---

API keys let clients fetch certificate material without administrator access.

## Scopes

| Scope | Allows |
|---|---|
| `certificates:read` | Certificate metadata, versions, `certificate.crt`, `chain.crt`, and `fullchain.crt` |
| `private_keys:read` | `private.key` downloads |
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
  https://certvault.example/api/v1/certificates/homelab/fullchain.crt \
  --output fullchain.crt
```

Private keys require `private_keys:read`:

```shell
curl --fail --silent --show-error \
  -H 'Authorization: Bearer cv_live_PREFIX.SECRET' \
  https://certvault.example/api/v1/certificates/homelab/private.key \
  --output private.key
```

Downloads include an `ETag`, which clients may send back through `If-None-Match`. CertVault returns `304 Not Modified` when the artifact is unchanged and does not create a download audit event for that response.

## Version-consistent bundles

Use one request when retrieving multiple artifacts:

```shell
curl --fail --silent --show-error \
  -H 'Authorization: Bearer cv_live_PREFIX.SECRET' \
  'https://certvault.example/api/v1/certificates/homelab/bundle.tar?files=fullchain.crt,private.key' \
  --output bundle.tar
```

The `files` parameter selects unique artifact names. Every entry comes from one resolved certificate version, even if renewal completes during the request. Each artifact requires its usual scope and certificate permission. Disabled certificates return 404, including conditional requests for cached bundles.

Bundles have a version-and-file-set ETag. Unchanged requests return 304 before reading artifacts or assembling an archive. Clients requesting the same version and file set share an 8 MiB bounded in-memory cache; decrypted bundles are never cached on server disk.

## Automatic download jobs

After creating a key, the console generates an installer command for Linux and Unix-like clients. The installer:

- Stores the API key in a mode `0600` file
- Retrieves all selected artifacts in one version-consistent bundle
- Verifies certificate and private-key public keys match when both are selected
- Locks the entire sync and reload across jobs sharing a destination
- Installs the selected cron schedule
- Performs the first download immediately
- Tracks one bundle `ETag`, saved only after deployment and reload succeed
- Uses conditional requests and avoids rewriting unchanged destination files
- Allows each artifact to be installed under a service-specific output name
- Optionally runs a reload command after a new bundle is deployed

The client requires Linux with `curl`, `tar`, OpenSSL, `flock`, and GNU `mv`. On ordinary filesystems, output filenames are symlinks into a version directory; an atomic switch of a shared current symlink deploys the set. The current and previous successful snapshots are retained. Initial migration of existing regular files to symlinks happens individually before reload.

Filesystems such as Proxmox pmxcfs use fixed-file copies. Handled copy or reload failures attempt rollback, and the ETag remains unacknowledged so the next run retries. This fallback cannot guarantee atomic replacement of the set or recovery from power loss or SIGKILL. Existing installed clients must rerun the installer to receive these changes.

Running the command again replaces the existing job for that certificate and selected artifact set.

Use repeated `--file ARTIFACT` options to select downloads, or
`--file ARTIFACT=OUTPUT` when a service requires a specific filename. Output
values must be plain filenames, not paths. An optional `--reload-command` runs
only after at least one downloaded artifact changes.
For example, a job installed on each Proxmox VE node can deploy the node-local
web interface certificate with:

```shell
curl -fsSL https://certvault.example/client/install.sh | \
  sudo env CERTVAULT_API_KEY='cv_live_PREFIX.SECRET' sh -s -- \
  --server 'https://certvault.example' \
  --certificate 'proxmox.example' \
  --file 'fullchain.crt=pveproxy-ssl.pem' \
  --file 'private.key=pveproxy-ssl.key' \
  --destination '/etc/pve/local' \
  --reload-command 'systemctl restart pveproxy' \
  --schedule '17 3 * * *'
```

## Optional client auto-update

The console's **Automatically update the client** checkbox is checked by default when accessed over HTTPS. For manually written installer commands, add `--auto-update`. Auto-update requires an HTTPS server URL and uses the existing certificate-job cron entries; it does not install another schedule. Existing clients need a one-time installer rerun to opt in.

```shell
curl --proto '=https' --proto-redir '=https' -fsSL https://certvault.example/client/install.sh | \
  sudo env CERTVAULT_API_KEY='cv_live_PREFIX.SECRET' sh -s -- \
  --server 'https://certvault.example' \
  --certificate 'homelab' \
  --file fullchain.crt --file private.key \
  --destination '/etc/ssl/certvault/homelab' \
  --reload-command 'systemctl reload nginx' \
  --auto-update
```

Each job calls a shared launcher. Jobs using the same server and client directories share the update check, signing-key pin, and installed client; different servers have separate update state. Each host still has one cron entry per certificate/artifact job. Update checks are throttled to once daily after success, with a one-hour retry delay after failure. The schedule determines when those checks can run.

The launcher checks for a new client first, then runs certificate sync with the installed version. A failed or interrupted update does not prevent sync. An unchanged client revision avoids downloading another archive. Client updates alone do not trigger service reloads; the existing certificate ETag and reload behavior control those.

Installation downloads and pins the server's public signing key over HTTPS. Later updates require a signed manifest, matching archive hash and size, supported protocol, regular script entries, and valid shell syntax. The bundle contains the launcher, updater, and sync script, so subsequent cron runs also use updated update logic. All update requests enforce HTTPS redirects and bounded network timeouts and send no certificate API token. The signing key identifies the configured CertVault server, rather than an independent upstream release publisher.

Verified scripts are installed in a new private version directory and an atomic symlink switch activates them. Concurrent jobs share an update lock; an active sync holds a shared run lock so updates cannot switch or prune its version. A busy sync postpones the update after a five-second lock timeout. The current and previous client versions are retained. Job tokens, artifact mappings, destinations, schedules, and reload commands remain separate from the update bundle.

The server stores `client-update-signing-key.enc` in its data directory, encrypted using its master key. Include that file and the master key in backups. Restarts preserve the public key. Replacing the key causes existing clients to reject updates; reinstalling does not silently replace their pin. To intentionally change trust, first verify the new key independently, then replace the affected `public.pem` under the client's `client-updates/<server-hash>` configuration directory. Set `next-check` there to `0` to request an immediate check on the next run.

To roll back locally, while jobs are stopped, point the client's `current` symlink to the target of its `previous` symlink and defer the next update check. To opt out, rerun the installer for that job without `--auto-update`; other opted-in jobs continue using their shared updater.

The deployed server release supplies `/client/update/key.pem`, `/client/update/manifest`, and `/client/update/<sha256>.tar`. These public endpoints serve only the signing public key and client code, including when the console UI is disabled. Client assets must be present in the configured UI directory; Docker images include them. Protocol 1 preserves the existing job arguments. A future incompatible protocol requires an installer rerun instead of executing an incompatible client.

## Durable renewal jobs

`POST /api/v1/certificates/{name}/renew` validates the configured, enabled certificate before acceptance. A 202 response contains `job_id` and `status`, with a `Location` header pointing to `/api/v1/jobs/{id}`. Requests for a certificate already queued or running return its existing job ID. The shared manual/scheduled queue admits at most 64 queued or running jobs; additional distinct requests receive 429 with `Retry-After: 60`. Missing, removed, and disabled certificates return 404.

Poll the job URL to observe `queued`, `running`, `succeeded`, or `failed`, including any failure message. Administrators can inspect all jobs; API keys need `renewals:trigger` or `certificates:read` and access to that job's certificate.

One worker processes persisted jobs in admission order, including when automatic issuance is disabled. Queued jobs survive restart. Running jobs interrupted by restart become failed because issuance may already have completed at the CA; these certificates are excluded from automatic scheduling until an administrator inspects the outcome and submits another renewal. Run one CertVault server per database; multiple independent workers sharing a database are unsupported.
