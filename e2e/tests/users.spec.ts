import * as path from "node:path"

import { expect, request, test } from "@playwright/test"

import { createUser, E2E_EMAIL, uniqueEmail } from "./helpers"

// User-admin lifecycle through the UI: create, edit/deactivate, and password
// reset. Emails use the e2e-…@tadmor.test pattern so global-teardown removes
// the rows (deactivation, not deletion, is the app's model).
test.describe("users", () => {
  test("create a user", async ({ page }) => {
    const email = uniqueEmail()

    await page.goto("/users/new")
    await expect(page.getByRole("heading", { name: "New User" })).toBeVisible()

    await page.getByLabel("Email").fill(email)
    await page.getByLabel("Full Name").fill("E2E Person")
    await page.getByLabel("Password", { exact: true }).fill("a-strong-password")
    await page.getByRole("button", { name: "Create" }).click()

    await expect(page).toHaveURL(/\/users$/)
    const row = page.getByRole("row").filter({ hasText: email })
    await expect(row).toBeVisible()
    await expect(row.getByText("E2E Person")).toBeVisible()
    await expect(row.getByText("Active")).toBeVisible()
    // The Administrator checkbox was left unchecked.
    await expect(row.getByText("User", { exact: true })).toBeVisible()
  })

  test("create an administrator", async ({ page }) => {
    const email = uniqueEmail()

    await page.goto("/users/new")
    await page.getByLabel("Email").fill(email)
    await page.getByLabel("Full Name").fill("E2E Admin")
    await page.getByLabel("Password", { exact: true }).fill("a-strong-password")
    await page.getByLabel("Administrator").check()
    await page.getByRole("button", { name: "Create" }).click()

    await expect(page).toHaveURL(/\/users$/)
    const row = page.getByRole("row").filter({ hasText: email })
    await expect(row.getByText("Admin", { exact: true })).toBeVisible()
  })

  test("edit and deactivate a user", async ({ page, request }) => {
    const email = uniqueEmail()
    const id = await createUser(request, email)

    await page.goto(`/users/${id}`)
    await expect(page.getByRole("heading", { name: "Edit User" })).toBeVisible()

    await page.getByLabel("Full Name").fill("E2E Renamed")
    await page.getByLabel("Active").uncheck()
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/users$/)
    const row = page.getByRole("row").filter({ hasText: email })
    await expect(row.getByText("E2E Renamed")).toBeVisible()
    await expect(row.getByText("Inactive")).toBeVisible()
  })

  test("reset a user's password", async ({ page, request }) => {
    const id = await createUser(request, uniqueEmail())

    await page.goto(`/users/${id}`)
    await page.getByLabel("New Password").fill("another-password")
    await page.getByRole("button", { name: "Reset password" }).click()

    await expect(page.getByRole("status")).toContainText("Password changed")
  })

  test("self-deactivation is refused", async ({ page }) => {
    // The signed-in run user edits its own row; the server refuses the save.
    await page.goto("/users")
    await page
      .getByRole("row")
      .filter({ hasText: E2E_EMAIL })
      .getByRole("link", { name: "Edit" })
      .click()

    await page.getByLabel("Active").uncheck()
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page.getByRole("alert")).toContainText(
      "cannot deactivate your own account",
    )
  })

  test("self-demotion is refused", async ({ page }) => {
    await page.goto("/users")
    await page
      .getByRole("row")
      .filter({ hasText: E2E_EMAIL })
      .getByRole("link", { name: "Edit" })
      .click()

    await page.getByLabel("Administrator").uncheck()
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page.getByRole("alert")).toContainText(
      "cannot remove your own administrator access",
    )
  })
})

// Non-administrators get the day-to-day app but no user administration: the
// nav entry disappears and the API refuses the screen outright.
test.describe("non-admin", () => {
  // Start logged out; this suite signs in as a freshly created non-admin.
  test.use({ storageState: { cookies: [], origins: [] } })

  test("non-admins do not see the Users screen", async ({ page, baseURL }) => {
    // Create the non-admin with the admin session the suite saved in setup
    // (this test's own context is logged out on purpose).
    const admin = await request.newContext({
      baseURL,
      storageState: path.join(__dirname, "..", ".auth", "state.json"),
    })
    const email = uniqueEmail()
    await createUser(admin, email)
    await admin.dispose()

    await page.goto("/")
    await page.getByLabel("Email").fill(email)
    await page.getByLabel("Password", { exact: true }).fill("e2e-password")
    await page.getByRole("button", { name: "Sign in" }).click()

    // The shell appears without the Users nav entry.
    await expect(
      page.getByRole("link", { name: "Chart of Accounts" }),
    ).toBeVisible()
    await expect(page.getByRole("link", { name: "Users" })).not.toBeVisible()

    // Deep-linking to the screen hits the API's 403.
    await page.goto("/users")
    await expect(page.getByRole("alert")).toContainText(
      "administrator access required",
    )
  })
})
