import { expect, test } from "@playwright/test"

import {
  apiPost,
  createBillDraft,
  createInvoiceDraft,
  purchaseFixture,
  salesFixture,
  uniqueCode,
} from "./helpers"

// The admin-only unpost action on posted documents: it reverses the journal
// entry and returns the document to draft, so it can be corrected and posted
// again. The document specs cover posting; this covers the reverse. Setup posts
// via the API (that path is already exercised elsewhere) so each test drives
// only the unpost through the UI. The e2e login user is an administrator, so
// the Unpost button is present. One AR document (invoice) and its AP mirror
// (bill) share the DocumentDetail code path.
test.describe("unpost", () => {
  test("unpost a posted invoice back to draft", async ({ page, request }) => {
    const f = await salesFixture(request)
    const number = uniqueCode()
    const invoiceId = await createInvoiceDraft(request, f, number)
    await apiPost(request, `/api/sales-invoices/${invoiceId}/post`)

    await page.goto(`/invoices/${invoiceId}`)
    await expect(page.getByText("posted", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: "Unpost" }).click()

    // Back to draft: the journal-entry link is gone and posting is offered again.
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(
      page.getByRole("link", { name: /journal entry #\d+/ }),
    ).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Post to ledger" }),
    ).toBeVisible()
    await expect(page.getByRole("button", { name: "Unpost" })).toHaveCount(0)
  })

  test("unpost a posted bill back to draft", async ({ page, request }) => {
    const f = await purchaseFixture(request)
    const number = uniqueCode()
    const billId = await createBillDraft(request, f, number)
    await apiPost(request, `/api/purchase-bills/${billId}/post`)

    await page.goto(`/bills/${billId}`)
    await expect(page.getByText("posted", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: "Unpost" }).click()

    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(
      page.getByRole("link", { name: /journal entry #\d+/ }),
    ).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Post to ledger" }),
    ).toBeVisible()
    await expect(page.getByRole("button", { name: "Unpost" })).toHaveCount(0)
  })
})
