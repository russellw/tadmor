import { expect, test } from "@playwright/test"

import {
  apiPost,
  createCustomerPaymentDraft,
  createInvoiceDraft,
  salesFixture,
  uniqueCode,
} from "./helpers"

// Customer-payment lifecycle through the UI: create a draft, then the real
// AR flow — post the payment and apply it to the customer's open invoice.
// The supplier side renders through the same PaymentForm/PaymentDetail
// components, so the customer flow covers the shared code path.
test.describe("customer payments", () => {
  test("create a draft payment", async ({ page, request }) => {
    const f = await salesFixture(request)

    await page.goto("/customer-payments/new")
    await expect(
      page.getByRole("heading", { name: "New Customer Payment" }),
    ).toBeVisible()

    await page.getByLabel("Customer").click()
    await page.getByRole("option", { name: f.orgName }).click()
    await page.getByLabel("Payment Date").fill(f.day)
    await page.getByLabel("Amount").fill("250")
    // Currency auto-fills from the customer (USD); the deposit account is
    // required before posting, so the form asks for it up front.
    await page.getByLabel("Deposit Account").click()
    await page.getByRole("option", { name: f.cashAccount.code }).click()
    await page.getByRole("button", { name: "Create draft" }).click()

    await expect(page).toHaveURL(/\/customer-payments\/\d+$/)
    await expect(
      page.getByRole("heading", { name: /Customer Payment #\d+/ }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    // The amount shows twice (amount and unapplied remainder) — both 250.00.
    await expect(page.getByText("250.00").first()).toBeVisible()
  })

  test("post a payment and apply it to an open invoice", async ({
    page,
    request,
  }) => {
    const f = await salesFixture(request)
    const invoiceNumber = uniqueCode()
    const invoiceId = await createInvoiceDraft(request, f, invoiceNumber)
    await apiPost(request, `/api/sales-invoices/${invoiceId}/post`)
    const paymentId = await createCustomerPaymentDraft(request, f, "40")

    await page.goto(`/customer-payments/${paymentId}`)
    await page.getByRole("button", { name: "Post to ledger" }).click()
    await expect(page.getByText("posted", { exact: true })).toBeVisible()
    await expect(
      page.getByRole("link", { name: /journal entry #\d+/ }),
    ).toBeVisible()

    // Apply allocates the posted payment to the customer's open invoices,
    // oldest first — here, all 40.00 onto the one open invoice.
    await page.getByRole("button", { name: "Apply" }).click()
    const applicationRow = page
      .getByRole("row")
      .filter({ hasText: invoiceNumber })
    await expect(applicationRow.getByText("40.00")).toBeVisible()
  })
})
