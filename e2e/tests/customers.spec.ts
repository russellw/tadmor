import { expect, test } from "@playwright/test"

import { createCustomer, createOrganization, uniqueOrgName } from "./helpers"

// Customer lifecycle through the UI: create, edit, and deactivate.
//
// Note on "delete": the app has no hard-delete for master data, by design — an
// accounting customer may carry ledger history, so the lifecycle end-state is
// deactivation (is_active = false), not removal. The third test exercises that
// as the faithful "delete" flow. Throwaway organizations created here are named
// with the E2E- prefix and removed after the run by global-teardown (via psql),
// since the app itself cannot delete them.
test.describe("customers", () => {
  test("create: give an organization a customer role", async ({
    page,
    request,
  }) => {
    const orgName = uniqueOrgName()
    await createOrganization(request, orgName)

    await page.goto("/customers/new")
    await expect(
      page.getByRole("heading", { name: "New Customer" }),
    ).toBeVisible()

    // Organization is a Radix Select (combobox) in create mode, offering only
    // organizations that don't already have a customer.
    await page.getByLabel("Organization").click()
    await page.getByRole("option", { name: orgName }).click()

    await page.getByLabel("Customer #").fill("CUST-E2E")
    await page.getByLabel("Currency").fill("USD")
    await page.getByRole("button", { name: "Create" }).click()

    // Lands back on the list with the new customer present in its org's row.
    await expect(page).toHaveURL(/\/customers$/)
    const row = page.getByRole("row").filter({ hasText: orgName })
    await expect(row).toBeVisible()
    await expect(row.getByText("CUST-E2E")).toBeVisible()
    await expect(row.getByText("USD")).toBeVisible()
  })

  test("edit: update a customer's terms", async ({ page, request }) => {
    const orgName = uniqueOrgName()
    const orgId = await createOrganization(request, orgName)
    const customerId = await createCustomer(request, orgId, {
      customer_number: "BEFORE",
      currency_code: "USD",
    })

    await page.goto(`/customers/${customerId}`)
    await expect(
      page.getByRole("heading", { name: "Edit Customer" }),
    ).toBeVisible()
    // Organization is read-only in edit mode (it is the customer's identity).
    await expect(page.getByText(orgName)).toBeVisible()

    await page.getByLabel("Customer #").fill("AFTER")
    await page.getByLabel("Currency").fill("EUR")
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/customers$/)
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
    const customerId = await createCustomer(request, orgId, {
      customer_number: "DEACTIVATE-ME",
    })

    await page.goto(`/customers/${customerId}`)
    await expect(
      page.getByRole("heading", { name: "Edit Customer" }),
    ).toBeVisible()

    // Unchecking Active is the lifecycle end-state — the closest thing to a
    // delete for an accounting entity.
    await page.getByLabel("Active").uncheck()
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/customers$/)
    const row = page.getByRole("row").filter({ hasText: orgName })
    await expect(row.getByText("Inactive")).toBeVisible()
  })
})
