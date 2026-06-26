import { expect, test } from "@playwright/test"

// Smoke tests: the app boots and the first screen renders. These assume the
// stack is already running (Vite dev server + Go API + Postgres) at BASE_URL —
// see e2e/README.md. They make no assertions about specific data, only that the
// shell and a representative screen mount.

test("chart of accounts screen loads", async ({ page }) => {
  await page.goto("/accounts")
  await expect(
    page.getByRole("heading", { name: "Chart of Accounts" }),
  ).toBeVisible()
})

test("primary nav links are present", async ({ page }) => {
  await page.goto("/")
  for (const label of [
    "Chart of Accounts",
    "Organizations",
    "Customers",
    "Suppliers",
    "Products",
  ]) {
    await expect(page.getByRole("link", { name: label })).toBeVisible()
  }
})
