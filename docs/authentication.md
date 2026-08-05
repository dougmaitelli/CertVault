# Authentication

## Bootstrap administrator

The bootstrap token provides local or break-glass administrator access. Supply it using one of:

- `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN`
- `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE`

The file form is recommended for container deployments.

## OpenID Connect

```yaml
auth:
  oidc:
    issuer_url: https://auth.example.com/realms/homelab
    client_id: certvault
    client_secret_file: /run/secrets/oidc_client_secret
    allowed_groups: [cert-admins]
```

The client secret can be supplied in three ways:

- Set its value with `CERTVAULT_OIDC_CLIENT_SECRET`.
- Set `CERTVAULT_OIDC_CLIENT_SECRET_FILE` to the path of a mounted secret file.
- Set `auth.oidc.client_secret_file` to that file path in YAML, as shown above.

The raw secret value cannot be placed directly in YAML. Use only one secret source.

Register this redirect URI with the OIDC provider:

```text
{server.public_url}/auth/callback
```

OIDC uses Authorization Code flow with PKCE. If `allowed_groups` is empty, every successfully authenticated identity becomes an administrator; configuring an allowlist is strongly recommended.

## Administrator and client boundaries

Web sessions are administrators. API keys are client identities constrained by scopes and certificate allowlists. API keys cannot access audit logs, ACME-account management, API-key management, or the administrator job-history endpoint.
