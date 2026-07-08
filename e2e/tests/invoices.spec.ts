import { expect, test } from "@playwright/test"

import {
  createInvoiceDraft,
  salesFixture,
  uniqueCode,
} from "./helpers"

// Sales-invoice lifecycle through the UI: create a draft with a free-form
// line, post a draft to the ledger, and delete a draft. Each test brings its
// own customer, GL accounts, and a one-day open period via salesFixture (see
// helpers.ts), so posting never depends on the dev database's calendar.
test.describe("invoices", () => {
  test("create a draft invoice with a free-form line", async ({
    page,
    request,
  }) => {
    const f = await salesFixture(request)
    const number = uniqueCode()

    await page.goto("/invoices/new")
    await expect(
      page.getByRole("heading", { name: "New Invoice" }),
    ).toBeVisible()

    await page.getByLabel("Customer").click()
    await page.getByRole("option", { name: f.orgName }).click()
    await page.getByLabel("Invoice #").fill(number)
    await page.getByLabel("Invoice Date").fill(f.day)
    // Currency auto-fills from the customer (USD). One free-form line: the
    // revenue account must be chosen since there is no product to fall back to.
    await page.getByLabel("Description").fill("E2E consulting")
    await page.getByLabel("Qty").fill("2")
    await page.getByLabel("Unit Price").fill("50")
    await page.getByLabel("Revenue Account").click()
    await page.getByRole("option", { name: f.revenueAccount.code }).click()
    await page.getByRole("button", { name: "Create draft" }).click()

    // Lands on the detail page: draft badge, the line, and the DB-computed total.
    await expect(page).toHaveURL(/\/invoices\/\d+$/)
    await expect(
      page.getByRole("heading", { name: `Invoice ${number}` }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(page.getByText("E2E consulting")).toBeVisible()
    const totalRow = page
      .getByRole("row")
      .filter({ has: page.getByRole("cell", { name: "Total", exact: true }) })
    await expect(totalRow.getByText("100.00")).toBeVisible()
  })

  test("post a draft invoice to the ledger", async ({ page, request }) => {
    const f = await salesFixture(request)
    const number = uniqueCode()
    const invoiceId = await createInvoiceDraft(request, f, number)

    await page.goto(`/invoices/${invoiceId}`)
    await expect(page.getByText("draft", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: "Post to ledger" }).click()

    // Posting writes the journal entry and links it from the header.
    await expect(page.getByText("posted", { exact: true })).toBeVisible()
    await expect(
      page.getByRole("link", { name: /journal entry #\d+/ }),
    ).toBeVisible()
    await expect(
      page.getByRole("button", { name: "Post to ledger" }),
    ).toHaveCount(0)
  })

  test("delete a draft invoice", async ({ page, request }) => {
    const f = await salesFixture(request)
    const number = uniqueCode()
    const invoiceId = await createInvoiceDraft(request, f, number)

    await page.goto(`/invoices/${invoiceId}`)
    await expect(
      page.getByRole("heading", { name: `Invoice ${number}` }),
    ).toBeVisible()

    // Drafts are the only deletable state; the button asks for confirmation.
    page.on("dialog", (dialog) => void dialog.accept())
    await page.getByRole("button", { name: "Delete" }).click()

    await expect(page).toHaveURL(/\/invoices$/)
    await expect(page.getByText(number)).toHaveCount(0)
  })
})
