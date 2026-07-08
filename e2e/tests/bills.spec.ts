import { expect, test } from "@playwright/test"

import { createBillDraft, purchaseFixture, uniqueCode } from "./helpers"

// Purchase-bill lifecycle through the UI — the AP mirror of invoices.spec.ts:
// create a draft with a free-form line (unit cost + expense account instead of
// price + revenue account), post a draft to the ledger, delete a draft.
test.describe("bills", () => {
  test("create a draft bill with a free-form line", async ({
    page,
    request,
  }) => {
    const f = await purchaseFixture(request)
    const number = uniqueCode()

    await page.goto("/bills/new")
    await expect(page.getByRole("heading", { name: "New Bill" })).toBeVisible()

    await page.getByLabel("Supplier").click()
    await page.getByRole("option", { name: f.orgName }).click()
    await page.getByLabel("Bill #").fill(number)
    await page.getByLabel("Bill Date").fill(f.day)
    await page.getByLabel("Description").fill("E2E office supplies")
    await page.getByLabel("Qty").fill("2")
    await page.getByLabel("Unit Cost").fill("50")
    await page.getByLabel("Expense Account").click()
    await page.getByRole("option", { name: f.expenseAccount.code }).click()
    await page.getByRole("button", { name: "Create draft" }).click()

    await expect(page).toHaveURL(/\/bills\/\d+$/)
    await expect(
      page.getByRole("heading", { name: `Bill ${number}` }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(page.getByText("E2E office supplies")).toBeVisible()
    const totalRow = page
      .getByRole("row")
      .filter({ has: page.getByRole("cell", { name: "Total", exact: true }) })
    await expect(totalRow.getByText("100.00")).toBeVisible()
  })

  test("post a draft bill to the ledger", async ({ page, request }) => {
    const f = await purchaseFixture(request)
    const billId = await createBillDraft(request, f, uniqueCode())

    await page.goto(`/bills/${billId}`)
    await expect(page.getByText("draft", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: "Post to ledger" }).click()

    await expect(page.getByText("posted", { exact: true })).toBeVisible()
    await expect(
      page.getByRole("link", { name: /journal entry #\d+/ }),
    ).toBeVisible()
    await expect(
      page.getByRole("button", { name: "Post to ledger" }),
    ).toHaveCount(0)
  })

  test("delete a draft bill", async ({ page, request }) => {
    const f = await purchaseFixture(request)
    const number = uniqueCode()
    const billId = await createBillDraft(request, f, number)

    await page.goto(`/bills/${billId}`)
    await expect(
      page.getByRole("heading", { name: `Bill ${number}` }),
    ).toBeVisible()

    page.on("dialog", (dialog) => void dialog.accept())
    await page.getByRole("button", { name: "Delete" }).click()

    await expect(page).toHaveURL(/\/bills$/)
    await expect(page.getByText(number)).toHaveCount(0)
  })
})
