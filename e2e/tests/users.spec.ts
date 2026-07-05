import { expect, test } from "@playwright/test"

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
    await page.getByLabel("Password").fill("a-strong-password")
    await page.getByRole("button", { name: "Create" }).click()

    await expect(page).toHaveURL(/\/users$/)
    const row = page.getByRole("row").filter({ hasText: email })
    await expect(row).toBeVisible()
    await expect(row.getByText("E2E Person")).toBeVisible()
    await expect(row.getByText("Active")).toBeVisible()
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
})
