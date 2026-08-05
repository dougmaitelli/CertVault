# Client access

API keys let clients fetch certificate material without administrator access.

## Scopes

| Scope | Allows |
|---|---|
| `certificates:read` | Certificate metadata, versions, `certificate.pem`, `chain.pem`, and `fullchain.pem` |
| `private_keys:read` | `private-key.pem` downloads |
| `renewals:trigger` | Manual renewal requests |

Every key also has a certificate allowlist. Selecting “Any certificate” includes certificates added later.

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
