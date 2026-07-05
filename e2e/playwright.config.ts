import { defineConfig, devices } from "@playwright/test"

// Where the running app is served. Default is the Vite dev server (which proxies
// /api → the Go backend on :8080). To test the embedded production build served
// by the Go binary directly, run with BASE_URL=http://localhost:8080.
const baseURL = process.env.BASE_URL ?? "http://localhost:5173"

export default defineConfig({
  testDir: "./tests",
  // Seeds the e2e login user and saves an authenticated storage state before
  // any test runs. See global-setup.ts.
  globalSetup: "./global-setup.ts",
  // Removes the throwaway org/customer rows the tests create (the app has no
  // hard-delete for master data) and the e2e login user. See global-teardown.ts.
  globalTeardown: "./global-teardown.ts",
  fullyParallel: true,
  // Fail the build if a test was left focused with test.only (CI hygiene).
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL,
    // The session cookie global-setup saved; without it every screen is the
    // login form. auth.spec.ts opts back out to test the form itself.
    storageState: "./.auth/state.json",
    headless: true,
    // Artifacts only when something goes wrong — keeps runs cheap and lets us
    // actually see a failing screen.
    screenshot: "only-on-failure",
    trace: "on-first-retry",
  },
  projects: [
    // Chromium only: Playwright's own version-matched build, installed via
    // `pnpm install-browser`. We do not pull Firefox/WebKit.
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
})
