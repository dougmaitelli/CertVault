---
title: API reference
description: Use CertVault's HTTP endpoints and download its OpenAPI specification.
---

The client-facing API uses bearer API keys:

```http
Authorization: Bearer cv_live_PREFIX.SECRET
```

## Client endpoints

| Method | Endpoint | Required scope |
|---|---|---|
| `GET` | `/api/v1/certificates` | `certificates:read` |
| `GET` | `/api/v1/certificates/{name}` | `certificates:read` |
| `GET` | `/api/v1/certificates/{name}/versions` | `certificates:read` |
| `GET` | `/api/v1/certificates/{name}/certificate.crt` | `certificates:read` |
| `GET` | `/api/v1/certificates/{name}/chain.crt` | `certificates:read` |
| `GET` | `/api/v1/certificates/{name}/fullchain.crt` | `certificates:read` |
| `GET` | `/api/v1/certificates/{name}/private.key` | `private_keys:read` |
| `POST` | `/api/v1/certificates/{name}/renew` | `renewals:trigger` |

All certificate endpoints also enforce the API key’s certificate allowlist.

## Public health endpoints

| Method | Endpoint |
|---|---|
| `GET` | `/api/v1/health` |
| `GET` | `/api/v1/ready` |

Administrator endpoints use browser sessions and are not available to API keys.

[Download the OpenAPI specification](/CertVault/openapi.yaml)
