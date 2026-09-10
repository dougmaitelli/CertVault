# Browser integration and visual regression tests

The suite builds the React app and serves it through the production Go HTTP
handler. Tests use real authentication, API validation, SQLite repositories,
account file storage, and browser controls. `backend/e2e/main.go` is a standalone
harness behind the `e2e` build tag; its reset endpoint is absent from production.

## Run and inspect failures

```sh
# From the repository root: canonical Linux environment, also used by CI
make test-e2e-docker

# From web/: select an individual test
pnpm test:e2e:docker --grep 'API key'

# Local debugging: Go, Node and installed Chromium are required
pnpm exec playwright install chromium
pnpm test:e2e
pnpm test:e2e:ui
pnpm exec playwright show-report
```

The Docker runner installs dependencies in a disposable volume, leaving host
node_modules, build output, and caches alone. It writes reports and screenshots
back to the checkout. Go compilation requires a C compiler for SQLite. Port 8099
must be free; startup fails if another process owns it. The harness ignores
application configuration and secrets from the environment, uses a temporary
data directory, and removes it on graceful shutdown.

CI runs `UI integration and screenshots` on every PR and push to master. Its
`ui-test-report` artifact includes the HTML report, expected/actual/diff images,
traces, and failure videos. Configure that job as a required branch check to
block merges on failures.

## Intentional visual changes

1. Run the suite and inspect the differences and behavioral assertions.
2. Update only affected baselines in the canonical container:

   ```sh
   pnpm test:e2e:docker --update-snapshots --grep 'certificate list'
   ```

3. Review the PNG changes under `web/e2e/snapshots/` and include them with the UI
   change. Run again without the update flag before committing.

Normal runs never create or update baselines. The direct `test:e2e:update`
command is for use within the canonical environment. Playwright and its Docker
browser image are pinned to the same version; update them together and review
new baselines when upgrading. Local screenshots on other distributions may
render differently.

## Coverage and boundaries

- Desktop Chromium at 1400×900, using CertVault's current dark theme.
- Login validation, session persistence and logout; certificate list/grid,
  details, previous versions, pending and empty states.
- Real mock-ACME renewal, persistence after reload, browser downloads, X.509
  domain verification, and matching decrypted private key.
- API-key form validation, creation, certificate scoping, denied private-key and
  administrator access, token shown only once, revocation and deletion.
- Install-command builder screenshots for defaults and a customized Caddy
  deployment; certificate selection, destination preservation, output names,
  schedules, reload commands, clipboard copying, file selection and shell quoting.
- ACME account listing, current-account protection in the UI, confirmation,
  cancellation, and persistent deletion of an old local registration.
- History and audit filtering, URL persistence, pagination, and empty results;
  API failure display, recovery after reload, and version-load errors.
- Twelve audit events spanning all ten supported actions, administrator and
  machine actors, multiple resources, download details, and source addresses.
- Console scrolling keeps the sidebar and background within the viewport;
  audit-footer visibility, full modal coverage, and scrolling in short dialogs.

Visual fixtures use fixed certificate metadata, IDs, timestamps and synthetic
non-authenticating API-key prefixes. The browser clock, locale and timezone are
fixed. Login uses the real bootstrap route; afterward a fresh database removes
the variable login audit timestamp while preserving the real session cookie.
No screenshot includes random tokens or newly issued certificate metadata.
The builder test creates a key through the real API and substitutes only its
displayed token with a synthetic value. Its screenshots capture the builder
itself; the separate lifecycle test verifies the original token's authentication.
Seeded visual certificate versions have metadata only; the download test issues
a real locally signed certificate through mock ACME before downloading it.

One worker and a fresh database per test prevent shared-state races. Retired
databases remain open until shutdown so any finishing asynchronous renewal stays
isolated from the next test. No scheduler, real CA, DNS provider, OIDC provider,
notification service, or hook runs. Browser exceptions and unexpected external
browser requests fail tests. API response interception is limited to explicit
failure tests and the builder's displayed-token substitution described above.

Screenshots wait for meaningful content and loaded fonts/images; Playwright
finishes finite animations and disables infinite ones, with zero differing pixels allowed (using Playwright's default
per-pixel color threshold). They do not replace assertions or cover other
browsers, mobile layouts, or real external integrations. The existing
`make screenshots` command remains a separate documentation screenshot tool.
