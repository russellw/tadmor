import { expect, test } from "@playwright/test"

import {
  createInvoiceDraft,
  createSalesOrderDraft,
  salesFixture,
  uniqueCode,
} from "./helpers"

// The email button on the printable-document screens. Sending is inert unless
// SMTP is configured, and the e2e server sets none, so a send reports 501
// "email sending is not configured" — which is exactly what lets us drive the
// whole button → panel → API path without a mail server. One document
// (invoice, via DocumentDetail) and one order (sales order, via OrderDetail)
// cover both host components; purchase-side documents share the same panel.
test.describe("email a document", () => {
  test("email button on an invoice surfaces the not-configured state", async ({
    page,
    request,
  }) => {
    const f = await salesFixture(request)
    const invoiceId = await createInvoiceDraft(request, f, uniqueCode())

    await page.goto(`/invoices/${invoiceId}`)
    await page.getByRole("button", { name: "Email" }).click()

    const panel = page.getByRole("form", { name: "Email" })
    // A recipient is required until organizations carry an email address.
    await panel.getByRole("button", { name: "Send" }).click()
    await expect(
      panel.getByText("Enter at least one recipient email address."),
    ).toBeVisible()

    await panel.getByLabel("To").fill("client@example.com")
    await panel.getByRole("button", { name: "Send" }).click()

    // No SMTP configured → the endpoint returns 501 and the panel shows it.
    await expect(
      panel.getByText("email sending is not configured"),
    ).toBeVisible()
  })

  test("email button on a sales order surfaces the not-configured state", async ({
    page,
    request,
  }) => {
    const f = await salesFixture(request)
    const orderId = await createSalesOrderDraft(request, f, uniqueCode())

    await page.goto(`/sales-orders/${orderId}`)
    await page.getByRole("button", { name: "Email" }).click()

    const panel = page.getByRole("form", { name: "Email" })
    await panel.getByLabel("To").fill("buyer@example.com")
    await panel.getByRole("button", { name: "Send" }).click()

    await expect(
      panel.getByText("email sending is not configured"),
    ).toBeVisible()
  })
})
