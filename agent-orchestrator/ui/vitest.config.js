import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [
    svelte({ hot: !process.env.VITEST }),
  ],
  // Force the browser export condition so Svelte 5 resolves its browser runtime
  // (svelte/src/index-client.js) instead of the server runtime.
  // Without this Vitest's Node.js resolver picks the "default" export which is
  // the SSR/server version, causing `mount() is not available on the server`.
  resolve: {
    conditions: ['browser'],
  },
  test: {
    globals:      true,
    environment:  'jsdom',
    setupFiles:   ['./src/__tests__/setup.js'],
    include:      ['src/**/*.{test,spec}.js'],
    exclude:      ['**/node_modules/**'],
    // Integration tests run separately via `pnpm test:integration`
    // — they are skipped automatically unless INTEGRATION_URL is set.
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html'],
      include:  ['src/**/*.{js,svelte}'],
      exclude:  ['src/__tests__/**'],
    },
  },
})
