---
title: Operations
description: Manage renewal, audit retention, hooks, and health checks.
---

## Renewal

CertVault checks for certificates due for renewal every six hours. `acme.automatic_issuance` controls automatic issuance globally, and each certificate may override it with `automatic_issuance`.

Only one issuance runs at a time because DNS-provider construction consumes process environment. A failed renewal leaves the previous certificate version untouched.

## Audit retention

Audit events are retained indefinitely by default. Enable automatic cleanup with:

```yaml
audit:
  retention: 90d
```

Expired events are removed once at startup and every 24 hours afterward.

## Hooks

Supported events are:

- `certificate.issued`
- `certificate.renewed`
- `certificate.failed`

Hook delivery runs asynchronously and does not block certificate issuance. The default timeout is 15 seconds.

The optional `certificates` list limits a hook to named certificates. Omit it or leave it empty to receive matching events for every configured certificate. Unknown names are rejected during configuration validation.

### Webhook example

```yaml
hooks:
  - name: certificate-automation
    type: webhook
    events:
      - certificate.issued
      - certificate.renewed
      - certificate.failed
    certificates:
      - homelab-wildcard
    url: https://automation.example.com/hooks/certvault
    secret_file: /run/secrets/certvault_webhook_secret
    timeout: 15s
```

CertVault sends an HTTP `POST` with `Content-Type: application/json`. A successful receiver must return a `2xx` response. An issued-certificate payload resembles:

```json
{
  "id": "evt_1770000000000000000",
  "event": "certificate.issued",
  "timestamp": "2026-02-02T02:40:00Z",
  "certificate": "homelab-wildcard",
  "version": {
    "id": 42,
    "certificate_name": "homelab-wildcard",
    "serial": "01A2B3C4",
    "not_before": "2026-02-02T02:39:00Z",
    "not_after": "2026-05-03T02:39:00Z"
  }
}
```

Failed events include an `error` field and may have a `null` version.

When `secret_file` is configured, the request includes:

```text
X-CertVault-Signature-256: sha256=<hex HMAC-SHA256 of body>
```

The receiver should calculate HMAC-SHA256 over the raw request body with the shared secret and compare signatures using a constant-time operation.

### Executable hook example

```yaml
hooks:
  - name: local-event-log
    type: exec
    events:
      - certificate.issued
      - certificate.renewed
      - certificate.failed
    certificates:
      - homelab-wildcard
    command: /hooks/record-event
    args:
      - /data/hook-events.jsonl
    timeout: 10s
```

Mount the executable read-only into the container:

```yaml
services:
  certvault:
    volumes:
      - ./hooks/record-event:/hooks/record-event:ro
```

An example `record-event` script is:

```sh
#!/bin/sh
set -eu

destination=${1:?destination is required}
printf '%s\n' "$CERTVAULT_EVENT_JSON" >> "$destination"
```

Make it executable before starting the container:

```shell
chmod 0755 hooks/record-event
```

Executable hooks run directly without an implicit shell. They receive only `PATH=/usr/local/bin:/usr/bin:/bin` and `CERTVAULT_EVENT_JSON`, and are terminated at the configured timeout. The standard image is intentionally minimal, so mount a self-contained executable or script and any additional tooling it requires.

## Health endpoints

- `/api/v1/health` reports process health and the running application version.
- `/api/v1/ready` verifies that required infrastructure is available.
