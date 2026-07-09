import { expect, test } from "@playwright/test"

import {
  createStockedPurchaseOrderDraft,
  createStockedSalesOrderDraft,
  receiveFixture,
  shipFixture,
  uniqueCode,
} from "./helpers"

// The stock axis of order fulfilment: shipping a sales order and receiving a
// purchase order create the corresponding stock movements (an issue for a ship,
// a receipt for a receive) against a chosen warehouse, moving the order's
// shipped/received status to full. Both orders carry a stocked-product line, so
// the shipping/receiving axis is live (free-form lines have none). The document
// axis (invoice/bill) is covered by orders.spec.ts / purchase-orders.spec.ts.
test.describe("order fulfilment — stock axis", () => {
  test("ship a sales order into a warehouse", async ({ page, request }) => {
    const f = await shipFixture(request)
    const orderId = await createStockedSalesOrderDraft(request, f, uniqueCode())

    await page.goto(`/sales-orders/${orderId}`)
    await page.getByRole("button", { name: "Confirm" }).click()
    await expect(page.getByText("open", { exact: true })).toBeVisible()

    // "Ship" opens the move panel; quantity defaults to the full outstanding.
    await page.getByRole("button", { name: "Ship" }).click()
    const panel = page.getByRole("form", { name: "Ship" })
    await panel.getByLabel("Warehouse").click()
    await page.getByRole("option", { name: f.warehouse.code }).click()
    await panel.getByLabel("Date").fill(f.day)
    await panel.getByRole("button", { name: "Ship" }).click()

    // A stock movement is created (linked in the confirmation note) and the
    // order's shipped axis reads fully shipped.
    await expect(
      page.getByRole("link", { name: /movement #\d+/ }),
    ).toBeVisible()
    await expect(page.getByText("shipped", { exact: true })).toBeVisible()
  })

  test("receive a purchase order into a warehouse", async ({
    page,
    request,
  }) => {
    const f = await receiveFixture(request)
    const orderId = await createStockedPurchaseOrderDraft(
      request,
      f,
      uniqueCode(),
    )

    await page.goto(`/purchase-orders/${orderId}`)
    await page.getByRole("button", { name: "Confirm" }).click()
    await expect(page.getByText("open", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: "Receive" }).click()
    const panel = page.getByRole("form", { name: "Receive" })
    await panel.getByLabel("Warehouse").click()
    await page.getByRole("option", { name: f.warehouse.code }).click()
    await panel.getByLabel("Date").fill(f.day)
    await panel.getByRole("button", { name: "Receive" }).click()

    await expect(
      page.getByRole("link", { name: /movement #\d+/ }),
    ).toBeVisible()
    await expect(page.getByText("received", { exact: true })).toBeVisible()
  })
})
