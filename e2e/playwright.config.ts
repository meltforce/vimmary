// Browser tests against a running vimmary with the preview fixtures and
// without an auth block. scripts/e2e starts the instance and sets
// E2E_BASE_URL; see that script.
import { defineConfig, devices } from "@playwright/test";

const baseURL = process.env.E2E_BASE_URL;
if (!baseURL) {
  throw new Error("E2E_BASE_URL is not set; run the tests through scripts/e2e");
}

export default defineConfig({
  testDir: "tests",
  outputDir: "test-results",
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
