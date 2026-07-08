import { expect, test } from "@playwright/test"

import {
  createPurchaseOrderDraft,
  purchaseFixture,
  uniqueCode,
} from "./helpers"

// Purchase-order lifecycle through the UI — the AP mirror of orders.spec.ts:
// create a draft with a free-form line (no price prefill on the purchase
// side), then confirm the order and bill it, which creates a draft purchase
// bill from the outstanding quantities.
test.describe("purchase orders", () => {
  test("create a draft purchase order", async ({ page, request }) => {
    const f = await purchaseFixture(request)
    const number = uniqueCode()

    await page.goto("/purchase-orders/new")
    await expect(
      page.getByRole("heading", { name: "New Purchase Order" }),
    ).toBeVisible()

    await page.getByLabel("Supplier").click()
    await page.getByRole("option", { name: f.orgName }).click()
    await page.getByLabel("Order #").fill(number)
    await page.getByLabel("Order Date").fill(f.day)
    await page.getByLabel("Description").fill("E2E ordered supplies")
    await page.getByLabel("Qty").fill("3")
    await page.getByLabel("Unit Cost").fill("20")
    await page.getByLabel("Expense Account").click()
    await page.getByRole("option", { name: f.expenseAccount.code }).click()
    await page.getByRole("button", { name: "Create draft" }).click()

    await expect(page).toHaveURL(/\/purchase-orders\/\d+$/)
    await expect(
      page.getByRole("heading", { name: `Purchase Order ${number}` }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(page.getByText("E2E ordered supplies")).toBeVisible()
  })

  test("confirm an order and bill it", async ({ page, request }) => {
    const f = await purchaseFixture(request)
    const orderId = await createPurchaseOrderDraft(request, f, uniqueCode())
    const billNumber = uniqueCode()

    await page.goto(`/purchase-orders/${orderId}`)
    await page.getByRole("button", { name: "Confirm" }).click()
    await expect(page.getByText("open", { exact: true })).toBeVisible()

    // "Create Bill" opens an inline panel; quantities default to what is
    // outstanding, so only the number and the in-period date are needed.
    await page.getByRole("button", { name: "Create Bill" }).click()
    const panel = page.getByRole("form", { name: "Create Bill" })
    await panel.getByLabel("Bill #").fill(billNumber)
    await panel.getByLabel("Date", { exact: true }).fill(f.day)
    await panel.getByRole("button", { name: "Create bill" }).click()

    // Lands on the new draft bill carrying the order's line.
    await expect(page).toHaveURL(/\/bills\/\d+$/)
    await expect(
      page.getByRole("heading", { name: `Bill ${billNumber}` }),
    ).toBeVisible()
    await expect(page.getByText("draft", { exact: true })).toBeVisible()
    await expect(page.getByText("E2E ordered line")).toBeVisible()
  })
})
