import { defineConfig, devices } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:3998'

export default defineConfig({
  testDir: './e2e/specs',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  // One backend and one database are shared, so the specs run in order.
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: BASE_URL,
    // English UI, so the assertions can quote the copy the English locale has.
    locale: 'en-US',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'bash e2e/server.sh',
    // Readiness, not liveness: the suite asserts /api/ready right away, and
    // that answer only turns 200 once the verifier has discovered the
    // provider. Polling the stronger endpoint is what makes the wait honest.
    url: `${BASE_URL}/api/ready`,
    reuseExistingServer: false,
    timeout: 180_000,
  },
})
