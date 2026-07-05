import { expect, test } from "@playwright/test"

import { E2E_EMAIL } from "./helpers"

// The login screen and session lifecycle, driven through the real form. The
// suite's shared storage state (an already-authenticated session) is discarded
// here so every test starts logged out.
test.use({ storageState: { cookies: [], origins: [] } })

test("unauthenticated visit shows the login screen, not the app", async ({
  page,
}) => {
  await page.goto("/accounts")
  await expect(page.getByRole("heading", { name: "Tadmor" })).toBeVisible()
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible()
  await expect(
    page.getByRole("link", { name: "Chart of Accounts" }),
  ).not.toBeVisible()
})

test("wrong password is rejected with an error", async ({ page }) => {
  await page.goto("/")
  await page.getByLabel("Email").fill(E2E_EMAIL)
  await page.getByLabel("Password").fill("definitely-not-the-password")
  await page.getByRole("button", { name: "Sign in" }).click()
  await expect(page.getByRole("alert")).toContainText(
    "invalid email or password",
  )
})

test("sign in and sign out round trip", async ({ page }) => {
  // global-setup hands this run's generated password to the workers.
  const password = process.env.E2E_PASSWORD
  expect(password, "E2E_PASSWORD missing; did global-setup run?").toBeTruthy()

  await page.goto("/")
  await page.getByLabel("Email").fill(E2E_EMAIL)
  await page.getByLabel("Password").fill(password!)
  await page.getByRole("button", { name: "Sign in" }).click()

  // The app shell appears, showing who is signed in.
  await expect(
    page.getByRole("link", { name: "Chart of Accounts" }),
  ).toBeVisible()
  await expect(page.getByText("E2E Test User")).toBeVisible()

  await page.getByRole("button", { name: "Sign out" }).click()
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible()
})
