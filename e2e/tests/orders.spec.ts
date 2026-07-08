import { expect, test } from "@playwright/test"

import { createSalesOrderDraft, salesFixture, uniqueCode } from "./helpers"

// Sales-order lifecycle through the UI: create a draft with a free-form line,
// then the fulfilment flow — confirm the order and invoice it, which creates
// a draft sales invoice from the outstanding quantities. Purchase orders
// render through the same OrderForm/OrderDetail components, so this covers
// the shared code path.
test.describe("sales orders", () => {
  test("create a draft sales order", async ({ page, request }) => {
    const f = await salesFixture(request)
    const number = uniqueCode()

    await page.goto("/sales-orders/new")
    await expect(
      page.getByRole("heading", { name: "New Sales Order" }),
    ).toBeVisible()

    await page.getByLabel("Customer").click()
    await page.getByRole("option", { name: f.orgName }).click()
    await page.getByLabel("Order #").fill(number)
    await page.getByLabel("Order Date").fill(f.day)
    await page.getByLabel("Description").fill("E2E ordered goods")
    await page.getByLabel("Qty").fill("3")
    await page.getByLabel("Unit Price").fill("20")
    await page.getByLabel("Revenue Account").click()
    await page.getByRole("option", { name: f.revenueAccount.code }).click()
    await page.getByRole("button", { name: "Create draft" }).click()

    await expect(page).toHaveURL(/\/sales-orders\/\d+$/)
    await expect(
      page.getByRole("heading", { name: `Sales Order ${number}` }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(page.getByText("E2E ordered goods")).toBeVisible()
  })

  test("confirm an order and invoice it", async ({ page, request }) => {
    const f = await salesFixture(request)
    const orderId = await createSalesOrderDraft(request, f, uniqueCode())
    const invoiceNumber = uniqueCode()

    await page.goto(`/sales-orders/${orderId}`)
    await page.getByRole("button", { name: "Confirm" }).click()
    // draft → open: the order becomes eligible for fulfilment.
    await expect(page.getByText("open", { exact: true })).toBeVisible()

    // "Create Invoice" opens an inline panel; quantities default to what is
    // outstanding, so only the number and the in-period date are needed.
    await page.getByRole("button", { name: "Create Invoice" }).click()
    const panel = page.getByRole("form", { name: "Create Invoice" })
    await panel.getByLabel("Invoice #").fill(invoiceNumber)
    await panel.getByLabel("Date", { exact: true }).fill(f.day)
    await panel.getByRole("button", { name: "Create invoice" }).click()

    // Lands on the new draft invoice carrying the order's line.
    await expect(page).toHaveURL(/\/invoices\/\d+$/)
    await expect(
      page.getByRole("heading", { name: `Invoice ${invoiceNumber}` }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(page.getByText("E2E ordered line")).toBeVisible()
  })
})
