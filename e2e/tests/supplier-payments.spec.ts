import { expect, test } from "@playwright/test"

import {
  apiPost,
  createBillDraft,
  createSupplierPaymentDraft,
  purchaseFixture,
  uniqueCode,
} from "./helpers"

// Supplier-payment lifecycle through the UI — the AP mirror of
// payments.spec.ts: create a draft (money out, from a payment account), then
// post one and apply it to the supplier's open bill.
test.describe("supplier payments", () => {
  test("create a draft payment", async ({ page, request }) => {
    const f = await purchaseFixture(request)

    await page.goto("/supplier-payments/new")
    await expect(
      page.getByRole("heading", { name: "New Supplier Payment" }),
    ).toBeVisible()

    await page.getByLabel("Supplier").click()
    await page.getByRole("option", { name: f.orgName }).click()
    await page.getByLabel("Payment Date").fill(f.day)
    await page.getByLabel("Amount").fill("250")
    await page.getByLabel("Payment Account").click()
    await page.getByRole("option", { name: f.cashAccount.code }).click()
    await page.getByRole("button", { name: "Create draft" }).click()

    await expect(page).toHaveURL(/\/supplier-payments\/\d+$/)
    await expect(
      page.getByRole("heading", { name: /Supplier Payment #\d+/ }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    // The amount shows twice (amount and unapplied remainder) — both 250.00.
    await expect(page.getByText("250.00").first()).toBeVisible()
  })

  test("post a payment and apply it to an open bill", async ({
    page,
    request,
  }) => {
    const f = await purchaseFixture(request)
    const billNumber = uniqueCode()
    const billId = await createBillDraft(request, f, billNumber)
    await apiPost(request, `/api/purchase-bills/${billId}/post`)
    const paymentId = await createSupplierPaymentDraft(request, f, "40")

    await page.goto(`/supplier-payments/${paymentId}`)
    await page.getByRole("button", { name: "Post to ledger" }).click()
    await expect(page.getByText("posted", { exact: true })).toBeVisible()
    await expect(
      page.getByRole("link", { name: /journal entry #\d+/ }),
    ).toBeVisible()

    await page.getByRole("button", { name: "Apply" }).click()
    const applicationRow = page
      .getByRole("row")
      .filter({ hasText: billNumber })
    await expect(applicationRow.getByText("40.00")).toBeVisible()
  })
})
