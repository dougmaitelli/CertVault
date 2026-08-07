# Authentication

## Bootstrap administrator

The bootstrap token is optional. When configured, it provides local or break-glass administrator access. Supply it using one of:

- `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN`
- `CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE`

The file form is recommended for container deployments.

To disable bootstrap login, omit both environment variables and `auth.bootstrap_token_file` from YAML. When OIDC is enabled, bootstrap remains available only as a secondary break-glass method. At least one authentication method must be configured to sign in to the web console.

Browser sessions last 8 hours by default. Configure `auth.session_duration` in YAML or set `CERTVAULT_SESSION_DURATION` to a duration such as `12h` or `1d` to override it. This lifetime applies to both bootstrap and OIDC sessions.

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
