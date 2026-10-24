import { execFileSync } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { createServer } from "node:https";
import { request as upstreamRequest } from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { expect, test } from "./fixtures";

test("HTTPS installer command offers signed client auto-update", async ({
  browser,
}) => {
  const directory = await mkdtemp(path.join(tmpdir(), "certvault-https-"));
  const keyPath = path.join(directory, "key.pem");
  const certPath = path.join(directory, "cert.pem");
  execFileSync(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-keyout",
      keyPath,
      "-out",
      certPath,
      "-subj",
      "/CN=localhost",
      "-days",
      "1",
    ],
    { stdio: "ignore", timeout: 10_000 },
  );
  const proxy = createServer(
    { key: await readFile(keyPath), cert: await readFile(certPath) },
    (incoming, outgoing) => {
      const upstream = upstreamRequest(
        {
          hostname: "127.0.0.1",
          port: 8099,
          method: incoming.method,
          path: incoming.url,
          headers: incoming.headers,
        },
        (response) => {
          outgoing.writeHead(response.statusCode ?? 502, response.headers);
          response.pipe(outgoing);
        },
      );
      upstream.on("error", () => {
        outgoing.writeHead(502);
        outgoing.end();
      });
      incoming.pipe(upstream);
    },
  );
  await new Promise<void>((resolve) => proxy.listen(0, "127.0.0.1", resolve));
  const address = proxy.address();
  if (address === null || typeof address === "string")
    throw new Error("HTTPS proxy did not bind");
  const origin = `https://127.0.0.1:${address.port}`;
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    const page = await context.newPage();
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(origin);
    await page
      .getByLabel("Break-glass administrator token")
      .fill("certvault-e2e-admin");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page.getByRole("status")).toHaveText("Operational");
    await page
      .getByRole("navigation")
      .getByRole("link", { name: "api keys", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Create API key", exact: true })
      .click();
    await page.getByLabel("Name", { exact: true }).fill("Auto-updating host");
    await page
      .getByLabel("Any certificate, including certificates added later")
      .check();
    await page.getByRole("button", { name: "Create key", exact: true }).click();
    const helper = page.locator(".api-usage-helper");
    const autoUpdate = helper.getByLabel(
      "Automatically update the client (requires HTTPS)",
    );
    await expect(autoUpdate).toBeEnabled();
    await expect(autoUpdate).toBeChecked();
    const command = helper.locator(".api-command code");
    await expect(command).toContainText("--auto-update");
    await expect(command).toContainText(
      "--proto '=https' --proto-redir '=https'",
    );
    await expect(command).toContainText(`--server '${origin}'`);
    await autoUpdate.uncheck();
    await expect(command).not.toContainText("--auto-update");
    expect(errors).toEqual([]);
  } finally {
    await context.close();
    proxy.closeAllConnections();
    await new Promise<void>((resolve, reject) =>
      proxy.close((error) => (error ? reject(error) : resolve())),
    );
    await rm(directory, { recursive: true, force: true });
  }
});
