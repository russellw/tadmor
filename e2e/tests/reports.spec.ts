import { expect, test } from "@playwright/test"

// Financial statements render and their date filters round-trip to the API.
// The suite never posts documents (posted journal entries can't be torn down),
// so data assertions use a range far in the past where nothing is ever posted:
// the empty-state copy proves the filtered fetch succeeded.
test.describe("financial statements", () => {
  test("profit and loss filters by date range", async ({ page }) => {
    await page.goto("/reports/profit-and-loss")
    await expect(
      page.getByRole("heading", { name: "Profit & Loss" }),
    ).toBeVisible()

    await page.getByLabel("From").fill("1990-01-01")
    await page.getByLabel("To").fill("1990-01-31")
    await expect(
      page.getByText("No posted revenue or expenses in this range."),
    ).toBeVisible()
  })

  test("balance sheet filters by as-of date", async ({ page }) => {
    await page.goto("/reports/balance-sheet")
    await expect(
      page.getByRole("heading", { name: "Balance Sheet" }),
    ).toBeVisible()

    await page.getByLabel("As of").fill("1990-01-01")
    await expect(
      page.getByText("Nothing posted on or before this date."),
    ).toBeVisible()
  })

  test("trial balance drills down to an account ledger", async ({ page }) => {
    await page.goto("/reports/trial-balance")
    // The seeded chart of accounts guarantees a Cash row; its name links to
    // the account's ledger.
    await page.getByRole("link", { name: "Cash", exact: true }).click()
    await expect(
      page.getByRole("heading", { name: /1000 — Cash/ }),
    ).toBeVisible()

    // The date filter round-trips: a range in the far past is always empty.
    await page.getByLabel("From").fill("1990-01-01")
    await page.getByLabel("To").fill("1990-01-31")
    await expect(
      page.getByText("No posted activity in this range."),
    ).toBeVisible()
  })
})
