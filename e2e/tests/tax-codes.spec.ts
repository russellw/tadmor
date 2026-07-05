import { expect, test } from "@playwright/test"

import { createTaxCode, uniqueCode } from "./helpers"

// Tax-code lifecycle through the UI: create, then edit + deactivate. Codes are
// E2E- prefixed so global-teardown removes the rows (tax codes have a natural
// primary key and no hard-delete in the app).
test.describe("tax codes", () => {
  test("create a tax code", async ({ page }) => {
    const code = uniqueCode()

    await page.goto("/tax-codes/new")
    await expect(
      page.getByRole("heading", { name: "New Tax Code" }),
    ).toBeVisible()

    // Enter the code lowercase; the form normalizes it to uppercase on save.
    await page.getByLabel("Code").fill(code.toLowerCase())
    await page.getByLabel("Name").fill("E2E standard rate")
    await page.getByLabel("Rate %").fill("8.25")
    await page.getByRole("button", { name: "Create" }).click()

    await expect(page).toHaveURL(/\/tax-codes$/)
    const row = page.getByRole("row").filter({ hasText: code })
    await expect(row).toBeVisible()
    await expect(row.getByText("E2E standard rate")).toBeVisible()
    // numeric(7,4) comes back with four decimals.
    await expect(row.getByText("8.2500")).toBeVisible()
    await expect(row.getByText("Active")).toBeVisible()
  })

  test("edit and deactivate a tax code", async ({ page, request }) => {
    const code = await createTaxCode(request, uniqueCode(), { rate: "5" })

    await page.goto(`/tax-codes/${code}`)
    await expect(
      page.getByRole("heading", { name: "Edit Tax Code" }),
    ).toBeVisible()
    // The code is the row's identity: shown, but not editable.
    await expect(page.getByText(code)).toBeVisible()
    await expect(page.getByLabel("Code")).toHaveCount(0)

    await page.getByLabel("Name").fill("E2E renamed rate")
    await page.getByLabel("Rate %").fill("7.5")
    await page.getByLabel("Active").uncheck()
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/tax-codes$/)
    const row = page.getByRole("row").filter({ hasText: code })
    await expect(row.getByText("E2E renamed rate")).toBeVisible()
    await expect(row.getByText("7.5000")).toBeVisible()
    await expect(row.getByText("Inactive")).toBeVisible()
  })
})
