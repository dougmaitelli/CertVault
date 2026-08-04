# Security policy

## Supported versions

CertVault is currently maintained on the latest release line. Security fixes are applied to the latest release and to `master`; older releases may not receive backports.

| Version | Supported |
| --- | --- |
| Latest release | Yes |
| `master` | Yes |
| Older releases | No guaranteed support |

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability and do not include certificates, private keys, API tokens, DNS credentials, private domain names, or other homelab details in public reports.

Use [GitHub private vulnerability reporting](https://github.com/dougmaitelli/CertVault/security/advisories/new) to provide:

- A description of the issue and its potential impact
- Affected versions or commit hashes
- Reproduction steps or a proof of concept using synthetic data
- Any suggested mitigation
- Whether the issue is already public or known to others

Please allow the maintainers a reasonable opportunity to investigate and release a fix before public disclosure. You should receive an initial acknowledgement within seven days. Validation, remediation, and disclosure timelines depend on severity and complexity and will be coordinated through the private advisory.

## Deployment security

CertVault stores and serves TLS private keys. Anyone who can administer the application, obtain a sufficiently privileged API key, read the master key, or modify an executable hook may be able to impersonate services using managed certificates.

- Put CertVault behind HTTPS before accessing it over a network. Its API can return private keys and must not be exposed over plaintext HTTP.
- Enable OIDC for normal administration and retain the bootstrap token only as a protected break-glass credential.
- Grant machine API keys only the required scopes and certificate access. Treat `private_keys:read` as highly privileged.
- Keep the master key separate from backups of the data directory. Possession of both permits decryption of stored private material; loss of the master key makes encrypted data unrecoverable.
- Supply DNS credentials and application secrets through protected environment or secret files. Never commit them to configuration files or source control.
- Restrict filesystem access to the persistent data directory and client-side private keys.
- Configure trusted proxies narrowly. Do not trust forwarded client-address headers from arbitrary networks.
- Treat executable hooks as privileged code. Mount approved hook programs read-only and avoid invoking a shell.
- Bind CertVault to a private interface or firewall it from untrusted networks unless an authenticated reverse proxy protects it.
- Keep CertVault, its base image, and its dependencies updated.

See the [README](README.md#data-and-security-model) for additional details about storage, API access, hooks, and proxy handling.
