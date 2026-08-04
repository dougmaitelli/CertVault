# Contributing to CertVault

Thank you for helping improve CertVault. Bug reports, documentation, tests, and focused code changes are welcome.

## Before you start

- Search existing issues and pull requests before opening a duplicate.
- Use the bug-report form for reproducible defects and the feature-request form for proposed behavior.
- Open an issue before beginning a large change so its design and scope can be discussed.
- Never include credentials, tokens, certificate private keys, real private domains, database files, or other sensitive homelab information in an issue, screenshot, fixture, or commit.
- Report suspected vulnerabilities privately according to [SECURITY.md](SECURITY.md).

## Development setup

Requirements:

- Go 1.25.10 or the version declared by `backend/go.mod`
- Node.js 24 or newer with npm
- OpenSSL for generation of local development secrets
- Docker only for container builds or containerized screenshot generation

Install dependencies and start the mock ACME development environment:

```sh
make dependencies
make dev
```

Open `http://localhost:8081` and authenticate with `certvault-dev-admin`. Development state is written under the ignored `.cache/dev` directory. Mock ACME mode issues locally signed certificates without contacting a certificate authority or DNS provider.

## Project structure

- `backend/` contains the Go API, authentication, ACME services, persistence, and encrypted certificate storage.
- `web/src/` contains the React console, organized into pages, reusable components, and dialogs.
- `config/` contains complete example and development YAML configurations.
- `docs/openapi.yaml` describes the machine-facing API.
- `web/public/client/install.sh` installs recurring certificate downloads on client machines.

Keep HTTP handlers thin, put ACME and certificate behavior in services, and isolate database access in the model-specific repositories. Reusable UI behavior belongs in components or dialogs rather than individual pages.

## Required checks

Run the full validation suite before opening a pull request:

```sh
make check
```

This verifies Go and frontend formatting, runs `go vet`, golangci-lint, ESLint, backend and frontend tests, type-checks the UI, and builds both production layers. Use `make format` and the relevant lint-fix command before manually correcting formatting issues.

New behavior should include tests at the closest useful layer. Security-sensitive behavior must be enforced by the backend even when the UI also prevents an invalid action.

For visible UI changes, regenerate or include screenshots when they materially help review:

```sh
make screenshots
```

Use `make screenshots-docker` when a local Playwright browser is unavailable.

## Pull requests

- Keep changes focused and avoid unrelated refactoring or formatting.
- Explain the problem, chosen solution, and meaningful tradeoffs.
- Call out changes to configuration, persistence, API compatibility, authentication, authorization, cryptography, secret handling, or hooks.
- Update examples, API documentation, and tests when behavior changes.
- Use reserved example domains such as `example.com`; never use private or personally operated domains in fixtures.
- Keep the branch current with `master` and ensure CI passes.

## Licensing

By submitting a contribution, you agree to license it under the [GNU Affero General Public License v3.0](LICENSE), the same license that applies to CertVault.

## Commits

Use concise imperative commit subjects, such as `Prevent deletion of current ACME account`. Release notes are generated from commit messages, so user-facing commits should describe the observable change clearly.
