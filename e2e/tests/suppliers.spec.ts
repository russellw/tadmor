import { expect, test } from "@playwright/test"

import { createOrganization, createSupplier, uniqueOrgName } from "./helpers"

// Supplier lifecycle through the UI: create, edit, and deactivate. Mirrors
// customers.spec.ts — a supplier is the same one-role-per-organization pattern
// on the AP side, so the same "no hard-delete, deactivate instead" reasoning
// applies (see the note there). Throwaway organizations carry the E2E- prefix
// and are removed by global-teardown.
test.describe("suppliers", () => {
  test("create: give an organization a supplier role", async ({
    page,
    request,
  }) => {
    const orgName = uniqueOrgName()
    await createOrganization(request, orgName)

    await page.goto("/suppliers/new")
    await expect(
      page.getByRole("heading", { name: "New Supplier" }),
    ).toBeVisible()

    // Organization is a Radix Select (combobox) in create mode, offering only
    // organizations that don't already have a supplier.
    await page.getByLabel("Organization").click()
    await page.getByRole("option", { name: orgName }).click()

    await page.getByLabel("Supplier #").fill("SUPP-E2E")
    await page.getByLabel("Currency").fill("USD")
    await page.getByRole("button", { name: "Create" }).click()

    // Lands back on the list with the new supplier present in its org's row.
    await expect(page).toHaveURL(/\/suppliers$/)
    const row = page.getByRole("row").filter({ hasText: orgName })
    await expect(row).toBeVisible()
    await expect(row.getByText("SUPP-E2E")).toBeVisible()
    await expect(row.getByText("USD")).toBeVisible()
  })

  test("edit: update a supplier's terms", async ({ page, request }) => {
    const orgName = uniqueOrgName()
    const orgId = await createOrganization(request, orgName)
    const supplierId = await createSupplier(request, orgId, {
      supplier_number: "BEFORE",
      currency_code: "USD",
    })

    await page.goto(`/suppliers/${supplierId}`)
    await expect(
      page.getByRole("heading", { name: "Edit Supplier" }),
    ).toBeVisible()
    // Organization is read-only in edit mode (it is the supplier's identity).
    await expect(page.getByText(orgName)).toBeVisible()

    await page.getByLabel("Supplier #").fill("AFTER")
    await page.getByLabel("Currency").fill("EUR")
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/suppliers$/)
    const row = page.getByRole("row").filter({ hasText: orgName })
    await expect(row.getByText("AFTER")).toBeVisible()
    await expect(row.getByText("EUR")).toBeVisible()
    await expect(row.getByText("BEFORE")).toHaveCount(0)
  })

  test('deactivate: the app\'s "delete" for master data', async ({
    page,
    request,
  }) => {
    const orgName = uniqueOrgName()
    const orgId = await createOrganization(request, orgName)
    const supplierId = await createSupplier(request, orgId, {
      supplier_number: "DEACTIVATE-ME",
    })

    await page.goto(`/suppliers/${supplierId}`)
    await expect(
      page.getByRole("heading", { name: "Edit Supplier" }),
    ).toBeVisible()

    await page.getByLabel("Active").uncheck()
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/suppliers$/)
    const row = page.getByRole("row").filter({ hasText: orgName })
    await expect(row.getByText("Inactive")).toBeVisible()
  })
})
