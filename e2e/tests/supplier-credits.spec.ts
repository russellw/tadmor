import { expect, test } from "@playwright/test"

import {
  apiPost,
  createBillDraft,
  createSupplierCreditDraft,
  purchaseFixture,
  uniqueCode,
} from "./helpers"

// Supplier-credit lifecycle through the UI — the AP mirror of
// credit-notes.spec.ts: create a draft with a free-form line, then post one
// and apply it against the supplier's open bill.
test.describe("supplier credits", () => {
  test("create a draft supplier credit", async ({ page, request }) => {
    const f = await purchaseFixture(request)
    const number = uniqueCode()

    await page.goto("/supplier-credits/new")
    await expect(
      page.getByRole("heading", { name: "New Supplier Credit" }),
    ).toBeVisible()

    await page.getByLabel("Supplier").click()
    await page.getByRole("option", { name: f.orgName }).click()
    await page.getByLabel("Credit Note #").fill(number)
    await page.getByLabel("Date", { exact: true }).fill(f.day)
    await page.getByLabel("Description").fill("E2E returned supplies")
    await page.getByLabel("Qty").fill("1")
    await page.getByLabel("Unit Cost").fill("30")
    await page.getByLabel("Expense Account").click()
    await page.getByRole("option", { name: f.expenseAccount.code }).click()
    await page.getByRole("button", { name: "Create draft" }).click()

    await expect(page).toHaveURL(/\/supplier-credits\/\d+$/)
    await expect(
      page.getByRole("heading", { name: `Supplier Credit ${number}` }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(page.getByText("E2E returned supplies")).toBeVisible()
  })

  test("post a supplier credit and apply it to an open bill", async ({
    page,
    request,
  }) => {
    const f = await purchaseFixture(request)
    const billNumber = uniqueCode()
    const billId = await createBillDraft(request, f, billNumber)
    await apiPost(request, `/api/purchase-bills/${billId}/post`)
    const creditId = await createSupplierCreditDraft(
      request,
      f,
      uniqueCode(),
      "30",
    )

    await page.goto(`/supplier-credits/${creditId}`)
    await page.getByRole("button", { name: "Post to ledger" }).click()
    await expect(page.getByText("posted", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: "Apply" }).click()
    const applicationRow = page
      .getByRole("row")
      .filter({ hasText: billNumber })
    await expect(applicationRow.getByText("30.00")).toBeVisible()
  })
})
