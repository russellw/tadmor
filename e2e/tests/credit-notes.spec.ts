import { expect, test } from "@playwright/test"

import {
  apiPost,
  createCreditNoteDraft,
  createInvoiceDraft,
  salesFixture,
  uniqueCode,
} from "./helpers"

// Sales-credit-note lifecycle through the UI: create a draft with a free-form
// line, then post one and apply it against the customer's open invoice. The
// purchase side (supplier credits) renders through the same generic form and
// DocumentDetail components, so this covers the shared code path.
test.describe("credit notes", () => {
  test("create a draft credit note", async ({ page, request }) => {
    const f = await salesFixture(request)
    const number = uniqueCode()

    await page.goto("/credit-notes/new")
    await expect(
      page.getByRole("heading", { name: "New Credit Note" }),
    ).toBeVisible()

    await page.getByLabel("Customer").click()
    await page.getByRole("option", { name: f.orgName }).click()
    await page.getByLabel("Credit Note #").fill(number)
    await page.getByLabel("Date", { exact: true }).fill(f.day)
    await page.getByLabel("Description").fill("E2E returned goods")
    await page.getByLabel("Qty").fill("1")
    await page.getByLabel("Unit Price").fill("30")
    await page.getByLabel("Revenue Account").click()
    await page.getByRole("option", { name: f.revenueAccount.code }).click()
    await page.getByRole("button", { name: "Create draft" }).click()

    await expect(page).toHaveURL(/\/credit-notes\/\d+$/)
    await expect(
      page.getByRole("heading", { name: `Credit Note ${number}` }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(page.getByText("E2E returned goods")).toBeVisible()
  })

  test("post a credit note and apply it to an open invoice", async ({
    page,
    request,
  }) => {
    const f = await salesFixture(request)
    const invoiceNumber = uniqueCode()
    const invoiceId = await createInvoiceDraft(request, f, invoiceNumber)
    await apiPost(request, `/api/sales-invoices/${invoiceId}/post`)
    const noteId = await createCreditNoteDraft(request, f, uniqueCode(), "30")

    await page.goto(`/credit-notes/${noteId}`)
    await page.getByRole("button", { name: "Post to ledger" }).click()
    await expect(page.getByText("posted", { exact: true })).toBeVisible()

    // Apply allocates the unapplied credit to the customer's open invoices,
    // oldest first; the allocation lists in the "Applied to" table.
    await page.getByRole("button", { name: "Apply" }).click()
    const applicationRow = page
      .getByRole("row")
      .filter({ hasText: invoiceNumber })
    await expect(applicationRow.getByText("30.00")).toBeVisible()
  })
})
