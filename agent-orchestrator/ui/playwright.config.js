import { defineConfig, devices } from '@playwright/test'

const baseURL = process.env.BASE_URL || 'http://localhost:8080'

// Path to the pre-built binary (relative to this config file, i.e. ui/).
// Build it first with: task build  (from the agent-orchestrator/ directory)
const ext = process.platform === 'win32' ? '.exe' : ''
const serverBin = `../bin/agent-orchestrator${ext}`

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'html',

  // Start the server if it isn't already running.
  // reuseExistingServer=true means: if something is already listening on
  // baseURL, skip the command entirely (supports `task start` workflows).
  webServer: {
    command: `${serverBin} server --config ../config.yaml`,
    url: baseURL,
    reuseExistingServer: true,
    timeout: 30_000,
  },

  use: {
    baseURL,
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
})
