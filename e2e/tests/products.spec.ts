import { expect, test } from "@playwright/test"

import { createProduct, uniqueCode } from "./helpers"

// Product lifecycle through the UI: create, edit, and deactivate. Same shape as
// customers.spec.ts, but products are standalone catalog entities (no
// organization role), keyed for the tests by an E2E-prefixed SKU that
// global-teardown removes. Deactivation is the "delete" here too — a product
// may be referenced by invoice lines.
test.describe("products", () => {
  test("create: add a product to the catalog", async ({ page }) => {
    const sku = uniqueCode()

    await page.goto("/products/new")
    await expect(
      page.getByRole("heading", { name: "New Product" }),
    ).toBeVisible()

    await page.getByLabel("SKU").fill(sku)
    await page.getByLabel("Name", { exact: true }).fill("E2E widget")
    await page.getByLabel("Unit Price").fill("12.50")
    await page.getByLabel("Currency").fill("USD")
    // The inventory-account fields only render once tracking is on; leave the
    // accounts at None — only the flag is asserted here (the Tracked badge).
    await page.getByLabel("Track inventory").check()
    await page.getByRole("button", { name: "Create" }).click()

    // Lands back on the list; unit_price is numeric(19,4), so 12.50 renders
    // with the full scale.
    await expect(page).toHaveURL(/\/products$/)
    const row = page.getByRole("row").filter({ hasText: sku })
    await expect(row).toBeVisible()
    await expect(row.getByText("E2E widget")).toBeVisible()
    await expect(row.getByText("12.5000")).toBeVisible()
    await expect(row.getByText("USD")).toBeVisible()
    await expect(row.getByText("Tracked")).toBeVisible()
  })

  test("edit: update a product's name and price", async ({
    page,
    request,
  }) => {
    const sku = uniqueCode()
    const productId = await createProduct(request, sku, "E2E before", {
      unit_price: "1",
    })

    await page.goto(`/products/${productId}`)
    await expect(
      page.getByRole("heading", { name: "Edit Product" }),
    ).toBeVisible()

    await page.getByLabel("Name", { exact: true }).fill("E2E after")
    await page.getByLabel("Unit Price").fill("99")
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/products$/)
    const row = page.getByRole("row").filter({ hasText: sku })
    await expect(row.getByText("E2E after")).toBeVisible()
    await expect(row.getByText("99.0000")).toBeVisible()
    await expect(row.getByText("E2E before")).toHaveCount(0)
  })

  test('deactivate: the app\'s "delete" for master data', async ({
    page,
    request,
  }) => {
    const sku = uniqueCode()
    const productId = await createProduct(request, sku, "E2E deactivate-me")

    await page.goto(`/products/${productId}`)
    await expect(
      page.getByRole("heading", { name: "Edit Product" }),
    ).toBeVisible()

    await page.getByLabel("Active", { exact: true }).uncheck()
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/products$/)
    const row = page.getByRole("row").filter({ hasText: sku })
    await expect(row.getByText("Inactive")).toBeVisible()
  })
})
