import { readFile } from "node:fs/promises";
import { X509Certificate, createPrivateKey } from "node:crypto";
import type { APIKeyCreationResponse } from "../src/api/types";
import { expect, login, navigate, screenshot, test } from "./fixtures";

test("login validation, session persistence and logout", async ({ page }) => {
  await page.goto("/certificates");
  await expect(page).toHaveURL("/");
  await expect(
    page.getByLabel("Break-glass administrator token"),
  ).toBeVisible();
  await screenshot(page, "login.png");

  await page.getByLabel("Break-glass administrator token").fill("invalid");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByText("Error: Invalid token", { exact: true }),
  ).toBeVisible();
  await screenshot(page, "login-error.png");

  await login(page);

  await page.reload();
  await expect(
    page.getByText("Bootstrap administrator", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(
    page.getByRole("button", { name: "Sign in", exact: true }),
  ).toBeVisible();

  await page.reload();
  expect((await page.request.get("/api/v1/session")).status()).toBe(401);
});

test("certificate list, grid, version history and pending state", async ({
  page,
}) => {
  await login(page);
  await screenshot(page, "certificates.png");

  await page.getByRole("button", { name: "Grid", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Grid", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await screenshot(page, "certificates-grid.png");

  await page
    .getByRole("heading", { name: "homelab-wildcard", exact: true })
    .click();
  await expect(
    page.getByText("Previous version", { exact: true }),
  ).toBeVisible();
  await screenshot(page, "certificate-details.png");

  await page.getByRole("button", { name: "Close", exact: true }).click();
  await page
    .getByRole("heading", { name: "internal-gateway", exact: true })
    .click();
  await expect(page.getByText("No certificate versions stored.")).toBeVisible();
  await expect(page.locator(".modal a[aria-disabled=true]")).toHaveCount(4);
  await screenshot(page, "certificate-pending.png");
});

test("renewal persists and downloads real certificate and private key", async ({
  page,
}) => {
  await login(page);

  const card = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: "internal-gateway", exact: true }),
  });
  await card.getByRole("button", { name: "Renew", exact: true }).click();
  await expect(card.getByText("valid", { exact: true })).toBeVisible();
  await expect(
    card.getByRole("button", { name: "Renew", exact: true }),
  ).toBeEnabled();

  await page.reload();
  await card.getByRole("heading").click();
  await expect(
    page.getByText("Current version", { exact: true }),
  ).toBeVisible();

  const artifacts: Buffer[] = [];

  for (const name of ["Certificate", "Private key"]) {
    const downloadEvent = page.waitForEvent("download");
    await page
      .locator(".modal")
      .getByRole("link", { name, exact: true })
      .click();
    const download = await downloadEvent;
    expect(await download.failure()).toBeNull();
    artifacts.push(await readFile(await download.path()));
  }

  const certificate = new X509Certificate(artifacts[0]);
  expect(certificate.subjectAltName).toContain("gateway.example.com");
  expect(
    certificate.checkPrivateKey(createPrivateKey(artifacts[1])),
  ).toBeTruthy();
  await page.getByRole("button", { name: "Close", exact: true }).click();

  await navigate(page, "history");
  await expect(
    page.getByRole("row").filter({ hasText: "internal-gateway" }),
  ).toContainText("succeeded");
});

test("API key form validation, scoped access, revoke and delete", async ({
  page,
  playwright,
  baseURL,
}) => {
  await login(page);

  await navigate(page, "api keys");
  await expect(page.locator("tbody tr")).toHaveCount(2);
  await screenshot(page, "api-keys.png");

  await page
    .getByRole("button", { name: "Create API key", exact: true })
    .click();
  await screenshot(page, "create-api-key.png");

  await page.getByRole("button", { name: "Create key", exact: true }).click();
  await expect(page.getByLabel("Name", { exact: true })).toBeFocused();
  expect(
    await page
      .getByLabel("Name", { exact: true })
      .evaluate((input: HTMLInputElement) => input.validity.valueMissing),
  ).toBeTruthy();

  await page.getByLabel("Name", { exact: true }).fill("Caddy deployment");
  await page.getByLabel("homelab-wildcard", { exact: true }).check();
  await page.getByLabel("Read private keys", { exact: true }).uncheck();
  await page.getByRole("button", { name: "Create key", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Automatic download", exact: true }),
  ).toBeVisible();

  await expect(
    page.getByRole("combobox", { name: /^Files/ }).locator("option"),
  ).toHaveText(["Full chain", "Certificate", "CA chain"]);
  await expect(
    page.getByRole("combobox", { name: /^Certificate/ }).locator("option"),
  ).toHaveText(["homelab-wildcard"]);

  const token = await page.locator(".token code").innerText();

  const machine = await playwright.request.newContext({
    baseURL,
    extraHTTPHeaders: { Authorization: `Bearer ${token}` },
  });

  try {
    const certificates = await machine.get("/api/v1/certificates");
    expect(certificates.ok()).toBeTruthy();
    expect(await certificates.json()).toEqual([
      expect.objectContaining({ name: "homelab-wildcard" }),
    ]);
    expect(
      (
        await machine.get("/api/v1/certificates/homelab-wildcard/private.key")
      ).status(),
    ).toBe(403);
    expect((await machine.get("/api/v1/api-keys")).status()).toBe(403);

    await page.reload();
    await expect(page.locator(".token")).toHaveCount(0);
    const row = page.getByRole("row").filter({ hasText: "Caddy deployment" });

    await row.getByRole("button", { name: "Revoke", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "Revoke API key?", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Revoke key", exact: true }).click();
    await expect(
      row.getByRole("button", { name: "Delete", exact: true }),
    ).toBeVisible();
    expect((await machine.get("/api/v1/certificates")).status()).toBe(401);

    await row.getByRole("button", { name: "Delete", exact: true }).click();
    await page.getByRole("button", { name: "Delete key", exact: true }).click();
    await expect(row).toHaveCount(0);

    await page.reload();
    await expect(page.locator("tbody tr")).toHaveCount(2);
  } finally {
    await machine.dispose();
  }
});

test("install command builder defaults, customization and copying", async ({
  page,
  context,
}) => {
  await login(page);
  await navigate(page, "api keys");

  // Create through the real API, replacing only the displayed random token.
  // The separate lifecycle test verifies authentication with an unmodified token.
  const displayToken = "cv_demo_synthetic_install_builder_token";
  await page.route("**/api/v1/api-keys", async (route) => {
    if (route.request().method() !== "POST") return route.continue();

    const response = await route.fetch();
    expect(response.status()).toBe(201);
    const result = (await response.json()) as APIKeyCreationResponse;
    await route.fulfill({ response, json: { ...result, token: displayToken } });
  });

  await page
    .getByRole("button", { name: "Create API key", exact: true })
    .click();
  await page.getByLabel("Name", { exact: true }).fill("Caddy deployment");
  await page
    .getByLabel("Any certificate, including certificates added later")
    .check();
  await page.getByRole("button", { name: "Create key", exact: true }).click();

  const helper = page.locator(".api-usage-helper");
  const command = helper.locator(".api-command code");
  await expect(
    helper.getByRole("heading", { name: "Automatic download" }),
  ).toBeVisible();
  await expect(
    helper.getByRole("combobox", { name: /^Certificate/ }),
  ).toHaveValue("homelab-wildcard");
  await expect(helper.getByRole("combobox", { name: /^Files/ })).toHaveValue(
    "fullchain.crt,private.key",
  );
  await expect(command).toContainText(
    "curl -fsSL 'http://127.0.0.1:8099/client/install.sh'",
  );
  await expect(command).toContainText(`CERTVAULT_API_KEY='${displayToken}'`);
  await expect(command).toContainText("--server 'http://127.0.0.1:8099'");
  await expect(command).toContainText("--certificate 'homelab-wildcard'");
  await expect(command).toContainText("--file 'fullchain.crt'");
  await expect(command).toContainText("--file 'private.key'");
  await expect(command).toContainText(
    "--destination '/etc/ssl/certvault/homelab-wildcard'",
  );
  await expect(command).toContainText("--schedule '17 3 * * *'");
  await expect(command).not.toContainText("--reload-command");
  await screenshot(page, "installation-command-default.png", helper);

  await helper
    .getByRole("combobox", { name: /^Certificate/ })
    .selectOption("monitoring-services");
  await expect(helper.getByLabel("Destination folder on client")).toHaveValue(
    "/etc/ssl/certvault/monitoring-services",
  );
  await helper
    .getByLabel("Destination folder on client")
    .fill("/etc/caddy/certificates");
  await helper
    .getByRole("combobox", { name: /^Certificate/ })
    .selectOption("homelab-wildcard");
  await expect(helper.getByLabel("Destination folder on client")).toHaveValue(
    "/etc/caddy/certificates",
  );
  await helper
    .getByLabel("Output name for fullchain.crt", { exact: true })
    .fill("caddy.crt");
  await helper
    .getByLabel("Output name for private.key", { exact: true })
    .fill("caddy.key");
  await helper
    .getByRole("combobox", { name: /^Schedule/ })
    .selectOption("17 * * * *");
  await helper
    .getByLabel("Command after files change (optional)")
    .fill("systemctl reload caddy");

  await expect(command).toContainText("--file 'fullchain.crt=caddy.crt'");
  await expect(command).toContainText("--file 'private.key=caddy.key'");
  await expect(command).toContainText(
    "--destination '/etc/caddy/certificates'",
  );
  await expect(command).toContainText("--schedule '17 * * * *'");
  await expect(command).toContainText(
    "--reload-command 'systemctl reload caddy'",
  );
  await screenshot(page, "installation-command-customized.png", helper);

  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await helper
    .getByRole("button", { name: "Copy command", exact: true })
    .click();
  await expect(
    helper.getByRole("button", { name: "Copied!", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    await command.innerText(),
  );

  await helper
    .getByRole("combobox", { name: /^Files/ })
    .selectOption("certificate.crt");
  await expect(
    helper.getByLabel("Output name for private.key", { exact: true }),
  ).toHaveCount(0);
  await expect(command).toContainText("--file 'certificate.crt'");
  await expect(command).not.toContainText("--file 'private.key");
  await helper
    .getByLabel("Destination folder on client")
    .fill("/srv/client's certs");
  await expect(command).toContainText(
    `--destination '/srv/client'"'"'s certs'`,
  );
});

test("ACME accounts protect current account and delete old registration", async ({
  page,
}) => {
  await login(page);

  await navigate(page, "ACME accounts");
  await expect(page.locator("article")).toHaveCount(2);
  await expect(
    page.getByText("https://acme.example.com/account/demo", { exact: true }),
  ).toBeVisible();
  await screenshot(page, "acme-accounts.png");

  await expect(
    page
      .locator("article.current")
      .getByRole("button", { name: "Delete", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await screenshot(page, "delete-acme-account.png");

  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.locator("article")).toHaveCount(2);
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page
    .getByRole("button", { name: "Delete account", exact: true })
    .click();
  await expect(page.locator("article")).toHaveCount(1);

  await page.reload();
  await expect(
    page.getByRole("heading", { name: "acme.example.com", exact: true }),
  ).toBeVisible();
  await expect(page.locator("article")).toHaveCount(1);
});

test("history filters and pagination persist in the URL", async ({ page }) => {
  await login(page);

  await navigate(page, "history");
  await expect(page.getByText("12 jobs", { exact: true })).toBeVisible();
  await screenshot(page, "history.png");

  await page.getByRole("combobox", { name: /^Rows/ }).selectOption("10");
  await expect(page.locator("tbody tr")).toHaveCount(10);

  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.locator("tbody tr")).toHaveCount(2);
  await expect(page).toHaveURL(/page=2/);

  await page.getByText("Statuses", { exact: true }).click();
  await page.getByLabel("failed", { exact: true }).check();
  await page
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(page.getByText("1 jobs", { exact: true })).toBeVisible();
  await expect(
    page.getByText("DNS challenge timed out", { exact: true }),
  ).toBeVisible();
  await screenshot(page, "history-filtered.png");

  await page.reload();
  await expect(page.locator("tbody tr")).toHaveCount(1);

  await page.goto("/history?status=running");
  await expect(
    page.getByText("No history entries match these filters."),
  ).toBeVisible();
  await screenshot(page, "history-empty.png");
});

test("audit log filters and pagination", async ({ page }) => {
  await login(page);

  await navigate(page, "audit logs");
  await expect(page.getByText("12 events", { exact: true })).toBeVisible();
  expect(
    new Set(await page.locator("tbody tr td:nth-child(3)").allTextContents())
      .size,
  ).toBe(10);
  expect(await page.evaluate(() => document.documentElement.scrollHeight)).toBe(
    900,
  );
  await screenshot(page, "audit-logs.png");

  const main = page.getByRole("main");
  await main.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(page.locator(".content-footer")).toBeInViewport();
  await expect(page.getByRole("navigation")).toBeInViewport();
  expect(await page.evaluate(() => window.scrollY)).toBe(0);
  await screenshot(page, "audit-logs-bottom.png");

  await page.getByRole("combobox", { name: /^Rows/ }).selectOption("10");
  await expect(page.locator("tbody tr")).toHaveCount(10);

  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.locator("tbody tr")).toHaveCount(2);

  await page.getByText("Actors", { exact: true }).click();
  await page.getByLabel("admin@example.com", { exact: true }).check();
  await page
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(page).toHaveURL(/actor=admin%40example.com/);
  await expect(page.locator("tbody tr")).toHaveCount(2);

  await page.reload();
  await expect(page.getByText("2 events", { exact: true })).toBeVisible();

  await page.getByText("Actions", { exact: true }).click();
  await page.getByLabel("renewal.trigger", { exact: true }).check();
  await page
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await expect(page.locator("tbody tr")).toContainText("homelab-wildcard");
  await screenshot(page, "audit-filtered.png");

  await page.goto("/audit-logs?actor=nobody");
  await expect(
    page.getByText("No audit events match these filters."),
  ).toBeVisible();
  await screenshot(page, "audit-empty.png");
});

test("empty console", async ({ page }) => {
  await login(page);
  expect(
    (await page.request.post("/__test/reset?scenario=empty")).ok(),
  ).toBeTruthy();

  await page.reload();
  await expect(page.getByRole("status")).toHaveText("Operational");
  await expect(page.locator("article")).toHaveCount(0);
  await screenshot(page, "certificates-empty.png");

  await navigate(page, "ACME accounts");
  await expect(
    page.getByText(/No ACME accounts have been created/),
  ).toBeVisible();
  await screenshot(page, "acme-accounts-empty.png");

  await navigate(page, "api keys");
  await expect(page.locator("tbody tr")).toHaveCount(0);
  await screenshot(page, "api-keys-empty.png");
});

test("API failure is visible and reload recovers", async ({ page }) => {
  await login(page);

  await page.route("**/api/v1/certificates", (route) =>
    route.fulfill({ status: 503, json: { detail: "Temporarily unavailable" } }),
  );

  await page.reload();
  await expect(
    page.getByText("Error: Temporarily unavailable", { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("status")).toHaveText("Operational");

  const banner = await page.locator("main > .error").boundingBox();
  const stats = await page.locator(".stats").boundingBox();
  expect(banner).not.toBeNull();
  expect(stats).not.toBeNull();
  expect(stats!.y - (banner!.y + banner!.height)).toBeGreaterThanOrEqual(24);
  await screenshot(page, "certificates-error.png");

  await page.unroute("**/api/v1/certificates");

  await page.reload();
  await expect(page.locator("article")).toHaveCount(3);
  await expect(
    page.getByText("Error: Temporarily unavailable", { exact: true }),
  ).toHaveCount(0);

  await page.route("**/api/v1/certificates/*/versions", (route) =>
    route.fulfill({ status: 503, json: { detail: "Unable to load versions" } }),
  );
  await page
    .getByRole("heading", { name: "homelab-wildcard", exact: true })
    .click();
  await expect(
    page.getByText("Unable to load versions", { exact: true }),
  ).toBeVisible();
  expect(await page.locator(".modal").boundingBox()).toEqual({
    x: 0,
    y: 0,
    width: 1400,
    height: 900,
  });
  expect(await page.evaluate(() => document.documentElement.scrollHeight)).toBe(
    900,
  );
  await screenshot(page, "certificate-versions-error.png");

  // A short viewport must keep the entire dialog reachable without exposing
  // or scrolling the content behind its backdrop.
  await page.setViewportSize({ width: 1400, height: 500 });

  const dialog = page.locator(".modal section");
  await dialog.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(
    page.getByText("Unable to load versions", { exact: true }),
  ).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollHeight)).toBe(
    500,
  );
  await dialog.evaluate((element) => element.scrollTo(0, 0));
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await expect(page.locator(".modal")).toHaveCount(0);
});
