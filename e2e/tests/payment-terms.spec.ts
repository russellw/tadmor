import { expect, test } from "@playwright/test"

import { createPaymentTerm, uniqueCode } from "./helpers"

// Payment-term lifecycle through the UI: create, then edit. Codes are E2E-
// prefixed so global-teardown removes the rows (payment terms have a natural
// primary key and no hard-delete in the app).
test.describe("payment terms", () => {
  test("create a payment term", async ({ page }) => {
    const code = uniqueCode()

    await page.goto("/payment-terms/new")
    await expect(
      page.getByRole("heading", { name: "New Payment Term" }),
    ).toBeVisible()

    // Enter the code lowercase; the form normalizes it to uppercase on save.
    await page.getByLabel("Code").fill(code.toLowerCase())
    // Name deliberately avoids the digits "45" so the due-days assertion below
    // matches exactly one cell.
    await page.getByLabel("Name").fill("E2E net terms")
    await page.getByLabel("Due Days").fill("45")
    await page.getByRole("button", { name: "Create" }).click()

    await expect(page).toHaveURL(/\/payment-terms$/)
    const row = page.getByRole("row").filter({ hasText: code })
    await expect(row).toBeVisible()
    await expect(row.getByText("E2E net terms")).toBeVisible()
    await expect(row.getByText("45")).toBeVisible()
  })

  test("edit a payment term", async ({ page, request }) => {
    const code = await createPaymentTerm(request, uniqueCode(), 30)

    await page.goto(`/payment-terms/${code}`)
    await expect(
      page.getByRole("heading", { name: "Edit Payment Term" }),
    ).toBeVisible()
    // The code is the row's identity: shown, but not editable.
    await expect(page.getByText(code)).toBeVisible()
    await expect(page.getByLabel("Code")).toHaveCount(0)

    await page.getByLabel("Name").fill("E2E renamed terms")
    await page.getByLabel("Due Days").fill("60")
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/payment-terms$/)
    const row = page.getByRole("row").filter({ hasText: code })
    await expect(row.getByText("E2E renamed terms")).toBeVisible()
    await expect(row.getByText("60")).toBeVisible()
  })

  test("rejects a negative due days", async ({ page }) => {
    await page.goto("/payment-terms/new")
    await page.getByLabel("Code").fill(uniqueCode())
    await page.getByLabel("Name").fill("E2E bad terms")
    await page.getByLabel("Due Days").fill("-5")
    await page.getByRole("button", { name: "Create" }).click()

    await expect(page.getByRole("alert")).toContainText("Due days")
    await expect(page).toHaveURL(/\/payment-terms\/new$/)
  })
})
