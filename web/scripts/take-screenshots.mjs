import { spawn } from "node:child_process";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const webDirectory = path.join(scriptDirectory, "..");
const repositoryDirectory = path.join(webDirectory, "..");
const screenshotDirectory = path.join(repositoryDirectory, "screenshots");
const port = 8089;
const baseURL = `http://127.0.0.1:${port}`;
const publicURL = "https://certvault.example.com";

const daysFromNow = (days) =>
  new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString();

const versions = {
  wildcard: [
    {
      id: 31,
      not_before: daysFromNow(-8),
      not_after: daysFromNow(82),
      created_at: daysFromNow(-8),
      domains: ["example.com", "*.example.com"],
      serial: "04:8A:B2:70:9F:36:DA:11",
      issuer: "Let's Encrypt R13",
      fingerprint_sha256:
        "7A:2B:94:03:C8:1E:6F:39:AE:04:9B:EC:95:89:40:7D:15:20:F1:8C:43:DE:AA:7D:30:9F:EF:C6:12:A8:E7:54",
    },
    {
      id: 22,
      not_before: daysFromNow(-96),
      not_after: daysFromNow(-6),
      created_at: daysFromNow(-96),
      domains: ["example.com", "*.example.com"],
      serial: "03:41:92:7C:68:DE:22:4A",
      issuer: "Let's Encrypt R12",
      fingerprint_sha256:
        "88:D1:A5:34:62:A4:BC:29:11:B7:7D:89:A2:21:37:3C:9F:6F:AA:35:5E:E4:13:43:CB:1A:D5:47:F4:2C:90:11",
    },
  ],
  services: [
    {
      id: 45,
      not_before: daysFromNow(-25),
      not_after: daysFromNow(65),
      created_at: daysFromNow(-25),
      domains: ["grafana.home.arpa", "prometheus.home.arpa"],
      serial: "04:73:FA:18:C2:9B:03:E5",
      issuer: "Let's Encrypt R13",
      fingerprint_sha256:
        "51:B9:45:E6:33:0A:C7:DE:BF:5F:81:B3:66:4E:2B:30:0D:60:4C:DF:0C:17:9A:A2:E4:5C:A9:C4:21:E7:30:68",
    },
  ],
};

const certificates = [
  {
    name: "homelab-wildcard",
    domains: ["example.com", "*.example.com"],
    key_type: "ec256",
    status: "valid",
    current_version: versions.wildcard[0],
  },
  {
    name: "monitoring-services",
    domains: ["grafana.home.arpa", "prometheus.home.arpa"],
    key_type: "ec384",
    status: "valid",
    current_version: versions.services[0],
  },
  {
    name: "internal-gateway",
    domains: ["gateway.example.com", "auth.example.com", "vault.example.com"],
    key_type: "rsa3072",
    status: "pending",
  },
];

const jobs = [
  {
    id: 18,
    certificate_name: "homelab-wildcard",
    kind: "renewal",
    status: "success",
    error: "",
    started_at: daysFromNow(-8),
    finished_at: daysFromNow(-8),
  },
  {
    id: 17,
    certificate_name: "monitoring-services",
    kind: "issuance",
    status: "success",
    error: "",
    started_at: daysFromNow(-25),
    finished_at: daysFromNow(-25),
  },
];

const apiKeys = [
  {
    id: 1,
    name: "Traefik deployment",
    prefix: "cv_live_3fa1",
    scopes: ["certificates:read", "private_keys:read"],
    certificates: ["homelab-wildcard"],
    created_at: daysFromNow(-41),
    last_used_at: daysFromNow(-1),
    revoked: false,
  },
  {
    id: 2,
    name: "Legacy NAS",
    prefix: "cv_live_90bd",
    scopes: ["certificates:read"],
    certificates: ["monitoring-services"],
    created_at: daysFromNow(-120),
    last_used_at: daysFromNow(-34),
    revoked: true,
  },
];

const accounts = [
  {
    id: "production-account",
    directory_url: "https://acme.example.com/directory",
    email: "admin@example.com",
    status: "valid",
    registration_url: "https://acme.example.com/acme/acct/production-demo",
    current: true,
  },
  {
    id: "staging-account",
    directory_url: "https://acme-staging.example.com/directory",
    email: "admin@example.com",
    status: "valid",
    registration_url: "https://acme-staging.example.com/acme/acct/staging-demo",
    current: false,
  },
];

const audits = [
  {
    id: 1,
    at: daysFromNow(-1),
    actor: "admin@example.com",
    action: "certificate.renewed",
    resource: "homelab-wildcard",
    detail: "ACME production renewal completed",
    ip: "192.168.1.24",
  },
];

async function waitForServer(timeout = 30_000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(baseURL);
      if (response.ok) return;
    } catch {
      // Vite is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`Vite did not start within ${timeout}ms`);
}

async function startVite() {
  const executable = path.join(webDirectory, "node_modules", ".bin", "vite");
  const viteProcess = spawn(
    executable,
    ["--host", "127.0.0.1", "--port", String(port)],
    {
      cwd: webDirectory,
      env: { ...globalThis.process.env, FORCE_COLOR: "0" },
      stdio: "inherit",
    },
  );
  await waitForServer();
  return () => viteProcess.kill("SIGTERM");
}

async function mockAPI(page) {
  await page.route(`${publicURL}/**`, async (route) => {
    const requestURL = new URL(route.request().url());
    const response = await route.fetch({
      url: `${baseURL}${requestURL.pathname}${requestURL.search}`,
    });
    await route.fulfill({ response });
  });

  await page.route("**/api/v1/**", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    const endpoint = pathname.replace("/api/v1/", "");

    if (endpoint === "session") {
      return route.fulfill({
        json: {
          name: "Doug Maitelli",
          email: "admin@example.com",
          authentication_method: "oidc",
          admin: true,
        },
      });
    }
    if (endpoint === "certificates")
      return route.fulfill({ json: certificates });
    if (endpoint === "jobs/history")
      return route.fulfill({
        json: {
          items: jobs,
          total: jobs.length,
          page: 1,
          per_page: 25,
          total_pages: 1,
          certificates: [
            ...new Set(jobs.map((job) => job.certificate_name)),
          ].sort(),
          operations: [...new Set(jobs.map((job) => job.kind))].sort(),
          statuses: [...new Set(jobs.map((job) => job.status))].sort(),
        },
      });
    if (endpoint === "api-keys" && route.request().method() === "POST") {
      return route.fulfill({
        json: {
          api_key: {
            id: 3,
            name: "Caddy deployment",
            prefix: "cv_live_demo",
            scopes: ["certificates:read", "private_keys:read"],
            certificates: ["*"],
            created_at: daysFromNow(0),
            last_used_at: null,
            revoked: false,
          },
          token: "cv_live_demo_synthetic_api_key_for_documentation",
        },
      });
    }
    if (endpoint === "api-keys") return route.fulfill({ json: apiKeys });
    if (endpoint === "acme-accounts" && route.request().method() === "GET") {
      return route.fulfill({ json: accounts });
    }
    if (endpoint === "audit") {
      return route.fulfill({
        json: {
          items: audits,
          total: audits.length,
          page: 1,
          per_page: 25,
          total_pages: 1,
          actors: [...new Set(audits.map((audit) => audit.actor))].sort(),
          actions: [...new Set(audits.map((audit) => audit.action))].sort(),
          resources: [...new Set(audits.map((audit) => audit.resource))].sort(),
        },
      });
    }
    if (endpoint === "health" || endpoint === "ready") {
      return route.fulfill({ json: { status: "ok" } });
    }
    if (endpoint === "certificates/homelab-wildcard/versions") {
      return route.fulfill({ json: versions.wildcard });
    }
    if (endpoint === "certificates/monitoring-services/versions") {
      return route.fulfill({ json: versions.services });
    }
    return route.fulfill({ status: 200, json: {} });
  });
}

async function capture(page, name) {
  const target = path.join(screenshotDirectory, name);
  await page.screenshot({ path: target });
  console.log(`Created ${path.relative(repositoryDirectory, target)}`);
}

async function main() {
  await mkdir(screenshotDirectory, { recursive: true });
  const stopVite = await startVite();
  let browser;

  try {
    browser = await chromium.launch({
      headless: true,
      args: ["--no-sandbox", "--disable-setuid-sandbox"],
    });
    const context = await browser.newContext({
      viewport: { width: 1400, height: 900 },
      colorScheme: "dark",
    });
    const page = await context.newPage();
    await mockAPI(page);

    await page.goto(`${publicURL}/certificates`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: "homelab-wildcard" }).waitFor();
    await capture(page, "certificates.png");

    await page.setViewportSize({ width: 1400, height: 1100 });
    await page.getByRole("heading", { name: "homelab-wildcard" }).click();
    await page.getByText("Previous version", { exact: true }).waitFor();
    await capture(page, "certificate-details.png");
    await page.setViewportSize({ width: 1400, height: 900 });

    await page.goto(`${publicURL}/acme-accounts`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: "acme.example.com" }).waitFor();
    await capture(page, "acme-accounts.png");

    await page.goto(`${publicURL}/api-keys`, { waitUntil: "networkidle" });
    await page.getByText("Traefik deployment").waitFor();
    await capture(page, "api-keys.png");

    await page.getByRole("button", { name: "Create API key" }).click();
    await page.getByLabel("Name").fill("Caddy deployment");
    await page
      .getByLabel("Any certificate, including certificates added later")
      .check();
    await page.getByRole("button", { name: "Create key" }).click();
    await page.getByRole("heading", { name: "Automatic download" }).waitFor();
    await page
      .getByLabel("Command after files change (optional)")
      .fill("systemctl reload caddy");
    await capture(page, "installation-command.png");
  } finally {
    await browser?.close();
    stopVite();
  }
}

main().catch((error) => {
  console.error(error);
  globalThis.process.exitCode = 1;
});
