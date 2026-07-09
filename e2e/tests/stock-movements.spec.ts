import { expect, test } from "@playwright/test"

import { createMovement, inventoryFixture } from "./helpers"

// Stock-movement lifecycle through the UI. A movement moves stock the moment it
// is created (there is no draft/posted status of its own); receipts and issues
// can then be posted to the ledger and, by an admin, unposted again. Each test
// brings its own stocked product, warehouse, clearing account, and a one-day
// open period via inventoryFixture (see helpers.ts), so posting never depends on
// the dev database's calendar.
test.describe("stock movements", () => {
  test("create a receipt through the form", async ({ page, request }) => {
    const f = await inventoryFixture(request)

    await page.goto("/stock-movements/new")
    await expect(
      page.getByRole("heading", { name: "New Stock Movement" }),
    ).toBeVisible()

    await page.getByLabel("Product").click()
    await page.getByRole("option", { name: f.productSku }).click()
    await page.getByLabel("Warehouse").click()
    await page.getByRole("option", { name: f.warehouse.code }).click()
    // Type defaults to "receipt". Quantity is a magnitude the type signs.
    await page.getByLabel("Date").fill(f.day)
    await page.getByLabel("Quantity").fill("10")
    await page.getByLabel("Unit Cost").fill("4")
    await page.getByRole("button", { name: "Create" }).click()

    // Lands on the detail page: a receipt movement, still a GL draft.
    await expect(page).toHaveURL(/\/stock-movements\/\d+$/)
    await expect(
      page.getByRole("heading", { name: /Stock Movement #\d+/ }),
    ).toBeVisible()
    await expect(page.getByText("receipt", { exact: true })).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
  })

  test("post a receipt to the ledger, then unpost it", async ({
    page,
    request,
  }) => {
    const f = await inventoryFixture(request)
    const movementId = await createMovement(request, f, "receipt", "10")

    await page.goto(`/stock-movements/${movementId}`)
    await expect(page.getByText("draft", { exact: true })).toBeVisible()

    // A receipt credits a chosen clearing account (typically GRNI) and debits
    // the product's inventory account.
    await page.getByLabel("Credit Account").click()
    await page.getByRole("option", { name: f.clearingAccount.code }).click()
    await page.getByRole("button", { name: "Post to ledger" }).click()

    // Posting writes the journal entry and links it from the header.
    await expect(page.getByText("posted", { exact: true })).toBeVisible()
    await expect(
      page.getByRole("link", { name: /journal entry #\d+/ }),
    ).toBeVisible()
    await expect(
      page.getByRole("button", { name: "Post to ledger" }),
    ).toHaveCount(0)

    // Unpost (admin-only) reverses the entry and returns the movement to draft.
    await page.getByRole("button", { name: "Unpost" }).click()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(
      page.getByRole("link", { name: /journal entry #\d+/ }),
    ).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Post to ledger" }),
    ).toBeVisible()
  })

  test("delete an unposted movement", async ({ page, request }) => {
    const f = await inventoryFixture(request)
    const movementId = await createMovement(request, f, "receipt", "10")

    await page.goto(`/stock-movements/${movementId}`)
    await expect(
      page.getByRole("heading", { name: `Stock Movement #${movementId}` }),
    ).toBeVisible()

    // Unposted movements are deletable; the button asks for confirmation.
    page.on("dialog", (dialog) => void dialog.accept())
    await page.getByRole("button", { name: "Delete" }).click()

    await expect(page).toHaveURL(/\/stock-movements$/)
  })
})
