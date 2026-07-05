import { expect, test } from "@playwright/test"

import { createWarehouse, uniqueCode } from "./helpers"

// Warehouse lifecycle through the UI: create, then edit + deactivate. Codes
// are E2E- prefixed so global-teardown removes the rows.
test.describe("warehouses", () => {
  test("create a warehouse", async ({ page }) => {
    const code = uniqueCode()

    await page.goto("/warehouses/new")
    await expect(
      page.getByRole("heading", { name: "New Warehouse" }),
    ).toBeVisible()

    // Enter the code lowercase; the form normalizes it to uppercase on save.
    await page.getByLabel("Code").fill(code.toLowerCase())
    await page.getByLabel("Name").fill("E2E overflow depot")
    await page.getByRole("button", { name: "Create" }).click()

    await expect(page).toHaveURL(/\/warehouses$/)
    const row = page.getByRole("row").filter({ hasText: code })
    await expect(row).toBeVisible()
    await expect(row.getByText("E2E overflow depot")).toBeVisible()
    await expect(row.getByText("Active")).toBeVisible()
  })

  test("edit and deactivate a warehouse", async ({ page, request }) => {
    const code = uniqueCode()
    const id = await createWarehouse(request, code, "E2E before")

    await page.goto(`/warehouses/${id}`)
    await expect(
      page.getByRole("heading", { name: "Edit Warehouse" }),
    ).toBeVisible()

    await page.getByLabel("Name").fill("E2E after")
    await page.getByLabel("Active").uncheck()
    await page.getByRole("button", { name: "Save" }).click()

    await expect(page).toHaveURL(/\/warehouses$/)
    const row = page.getByRole("row").filter({ hasText: code })
    await expect(row.getByText("E2E after")).toBeVisible()
    await expect(row.getByText("Inactive")).toBeVisible()
  })
})
