import {
  test as base,
  expect,
  type Locator,
  type Page,
} from "@playwright/test";

export const test = base.extend<{ guard: void }>({
  guard: [
    async ({ page, request, baseURL }, use) => {
      expect((await request.post("/__test/reset")).ok()).toBeTruthy();
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.route("**/*", async (route) => {
        const url = route.request().url();
        if (url.startsWith(baseURL + "/") || url.startsWith("data:"))
          return route.continue();
        errors.push(`Unexpected external request: ${url}`);
        await route.abort();
      });
      await page.clock.setFixedTime(new Date("2026-08-16T12:00:00Z"));
      await use();
      expect(errors, "Browser exceptions and external requests").toEqual([]);
    },
    { auto: true },
  ],
});
export { expect };

export async function login(page: Page) {
  await page.goto("/");
  await page
    .getByLabel("Break-glass administrator token")
    .fill("certvault-e2e-admin");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("navigation")).toBeVisible();
  // Keep the real session cookie, but reset the login audit and all other data
  // before visual assertions. Session signing uses the same test-only key.
  expect((await page.request.post("/__test/reset")).ok()).toBeTruthy();
  await page.reload();
  await expect(page.getByRole("status")).toHaveText("Operational");
  await expect(page.locator("article")).toHaveCount(3);
}

export async function navigate(page: Page, name: string) {
  await page
    .getByRole("navigation")
    .getByRole("link", { name, exact: true })
    .click();
  await expect(
    page.getByRole("heading", { level: 1, name, exact: true }),
  ).toBeVisible();
}

export async function screenshot(page: Page, name: string, target?: Locator) {
  await page.evaluate(() => document.fonts.ready);
  await page.evaluate(async () => {
    await Promise.all(
      Array.from(document.images).map((image) => image.decode()),
    );
  });
  await page.mouse.move(0, 0);
  if (target) {
    await expect(target).toHaveScreenshot(name);
  } else {
    await expect(page).toHaveScreenshot(name, { fullPage: true });
  }
}
