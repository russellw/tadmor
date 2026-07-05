import { expect, test } from "@playwright/test"

import {
  createFiscalYear,
  createPeriod,
  E2E_PREFIX,
  uniqueFarYear,
} from "./helpers"

// The fiscal-calendar screen: create a fiscal year, add a period to it, and
// close/reopen a period (the gate that decides whether documents can post).
//
// Every test uses its own random far-future year because accounting periods
// carry a global no-overlap constraint — dates must clash neither with the
// dev database's real calendar nor with other runs. Fiscal years are named
// with the E2E- prefix so global-teardown deletes them (periods first).
test.describe("periods", () => {
  test("create a fiscal year", async ({ page }) => {
    const year = uniqueFarYear()
    const name = `${E2E_PREFIX}FY-${year}-${Date.now()}`

    await page.goto("/fiscal-years/new")
    await expect(
      page.getByRole("heading", { name: "New Fiscal Year" }),
    ).toBeVisible()

    // The form prefills the year after the latest existing fiscal year;
    // overwrite everything with this test's own far-future year.
    await page.getByLabel("Name").fill(name)
    await page.getByLabel("Start").fill(`${year}-01-01`)
    await page.getByLabel("End").fill(`${year}-12-31`)
    await page.getByRole("button", { name: "Create" }).click()

    // Lands back on the periods screen with the new year's (empty) section.
    await expect(page).toHaveURL(/\/periods$/)
    await expect(page.getByRole("heading", { name })).toBeVisible()
    await expect(page.getByText(`${year}-01-01 → ${year}-12-31`)).toBeVisible()
  })

  test("add a period to a fiscal year", async ({ page, request }) => {
    const year = uniqueFarYear()
    const fyName = `${E2E_PREFIX}FY-${year}-${Date.now()}`
    await createFiscalYear(request, fyName, `${year}-01-01`, `${year}-12-31`)

    await page.goto("/periods/new")
    await expect(page.getByRole("heading", { name: "New Period" })).toBeVisible()

    // Fiscal year is a Radix Select (combobox); the prefill targets the month
    // after the latest real period, so point everything at this test's year.
    await page.getByLabel("Fiscal Year").click()
    await page.getByRole("option", { name: fyName }).click()
    await page.getByLabel("Name").fill(`${year}-03`)
    await page.getByLabel("Start").fill(`${year}-03-01`)
    await page.getByLabel("End").fill(`${year}-03-31`)
    await page.getByRole("button", { name: "Create" }).click()

    await expect(page).toHaveURL(/\/periods$/)
    const row = page.getByRole("row").filter({ hasText: `${year}-03` })
    await expect(row).toBeVisible()
    await expect(row.getByText("open")).toBeVisible()
  })

  test("close and reopen a period", async ({ page, request }) => {
    const year = uniqueFarYear()
    const fyName = `${E2E_PREFIX}FY-${year}-${Date.now()}`
    const fyId = await createFiscalYear(
      request,
      fyName,
      `${year}-01-01`,
      `${year}-12-31`,
    )
    await createPeriod(
      request,
      fyId,
      `${year}-05`,
      `${year}-05-01`,
      `${year}-05-31`,
    )

    await page.goto("/periods")
    const row = page.getByRole("row").filter({ hasText: `${year}-05` })
    await expect(row.getByText("open")).toBeVisible()

    // Close it (the inline toggle, not the edit form)…
    await row.getByRole("button", { name: "Close" }).click()
    await expect(row.getByText("closed")).toBeVisible()
    // …and reopen it.
    await row.getByRole("button", { name: "Reopen" }).click()
    await expect(row.getByText("open")).toBeVisible()
  })
})
