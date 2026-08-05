---
hide:
  - navigation
  - toc
---

<section class="cv-hero">
  <div class="cv-hero-copy">
    <span class="cv-eyebrow">Self-hosted certificate control</span>
    <h1>One vault for every TLS certificate.</h1>
    <p>Issue, renew, protect, and distribute certificates across your homelab from one focused control plane.</p>
    <div class="cv-actions">
      <a href="getting-started/" class="md-button md-button--primary">Get started</a>
      <a href="https://github.com/dougmaitelli/CertVault" class="md-button">View on GitHub</a>
    </div>
    <div class="cv-trust-row">
      <span>DNS-01 automation</span>
      <span>Encrypted at rest</span>
      <span>Scoped client access</span>
    </div>
  </div>
  <div class="cv-hero-mark" aria-hidden="true">
    <div class="cv-glow"></div>
    <div class="cv-vault-icon">CV</div>
  </div>
</section>

<section class="cv-feature-grid">
  <article class="cv-feature-card">
    <span class="cv-feature-number">01</span>
    <h2>Automated issuance</h2>
    <p>Obtain wildcard and multi-domain certificates through any DNS provider supported by lego.</p>
  </article>
  <article class="cv-feature-card">
    <span class="cv-feature-number">02</span>
    <h2>Protected material</h2>
    <p>Keep ACME account keys and certificate private keys authenticated-encrypted at rest.</p>
  </article>
  <article class="cv-feature-card">
    <span class="cv-feature-number">03</span>
    <h2>Controlled delivery</h2>
    <p>Give clients only the operations and certificate artifacts they are explicitly allowed to use.</p>
  </article>
</section>

<section class="cv-showcase">
  <div class="cv-showcase-copy">
    <span class="cv-eyebrow">Operational clarity</span>
    <h2>See certificate health at a glance.</h2>
    <p>The responsive console brings validity, renewal activity, version history, ACME accounts, API keys, and audit events into one place.</p>
    <a href="operations/">Explore operations →</a>
  </div>
  <div class="cv-screenshot-frame">
    <img src="https://raw.githubusercontent.com/dougmaitelli/CertVault/master/screenshots/certificates.png" alt="CertVault certificate inventory dashboard" loading="lazy">
  </div>
</section>

<section class="cv-architecture">
  <span class="cv-eyebrow">Simple by design</span>
  <h2>A focused control plane for your homelab.</h2>

```text
DNS provider ── DNS-01 ──> CertVault ── encrypted versions ──> persistent volume
                              │
                              ├── web console for administrators
                              └── scoped HTTPS API for certificate consumers
```
</section>

!!! warning
    CertVault can return private keys. Do not expose it over plaintext HTTP outside a trusted local development environment.
