---
title: Notifications
description: Send certificate issuance, renewal, and failure alerts through Apprise.
---

CertVault can send alerts through the [Apprise REST API](https://github.com/caronc/apprise-api). This provides delivery to Slack, Discord, Telegram, email, and the other services supported by Apprise.

Notifications are emitted when a certificate is initially issued, renewed, or cannot be issued. Delivery is asynchronous: an unavailable notification service is logged but never changes the certificate job result.

## Configure Apprise

Set the complete stateful Apprise notification endpoint in YAML:

```yaml
notifications:
  apprise_url: http://apprise:8000/notify/certvault
  apprise_tags: [admin, homelab]
  apprise_urls:
    - discord://webhook_id/webhook_token
```

The final path segment of a stateful endpoint is its Apprise configuration key. Tags select matching endpoints in that configuration. Inline URLs add delivery targets directly. Tags and inline URLs are optional and can be combined.

The equivalent environment variables are:

```properties
CERTVAULT_APPRISE_URL=http://apprise:8000/notify/certvault
CERTVAULT_APPRISE_TAGS=admin,homelab
CERTVAULT_APPRISE_URLS=discord://webhook_id/webhook_token
```

Requests time out after five seconds. CertVault sends the Apprise-compatible `title`, `body`, and `type` fields, plus `tag` and `urls` when configured.

:::caution
Apprise endpoints and inline notification URLs can contain credentials. Store them in deployment secrets, keep them out of source control, and do not include them in logs, screenshots, or issue reports.
:::
