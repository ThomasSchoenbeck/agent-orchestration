import { defineConfig, devices } from '@playwright/test'

// The server runs HTTPS by default (self-signed cert under {storage.root}/tls),
// so default to https and ignore the self-signed cert below. Override with
// BASE_URL, e.g. run the server with --insecure and set BASE_URL=http://localhost:8080.
const baseURL = process.env.BASE_URL || 'https://localhost:8080'

// Path to the pre-built binary (relative to this config file, i.e. ui/).
// Build it first with: task build  (from the agent-orchestrator/ directory)
const ext = process.platform === 'win32' ? '.exe' : ''
const serverBin = `../bin/agent-orchestrator${ext}`

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  // Always include 'list' so the run is never silent (the bare 'github' reporter
  // emits nothing to stdout outside GitHub Actions — and some shells set CI=1).
  reporter: process.env.CI
    ? [['github'], ['list']]
    : [['list'], ['html', { open: 'never' }]],

  // By default, connect to an already-running server at baseURL (the usual dev
  // workflow). In CI — or when PW_START_SERVER=1 — Playwright builds/starts the
  // server itself. This avoids the readiness-probe hang when a live server is up.
  webServer:
    process.env.CI || process.env.PW_START_SERVER === '1'
      ? {
          command: `${serverBin} server --config ../config.yaml`,
          url: baseURL,
          reuseExistingServer: true,
          ignoreHTTPSErrors: true, // self-signed dev cert
          timeout: 30_000,
        }
      : undefined,

  use: {
    baseURL,
    ignoreHTTPSErrors: true, // self-signed dev cert (browser + request contexts)
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
})
