import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  // The instance admin and the instance settings are set up once; after that
  // every test has users and administrations of its own, so they run side by side.
  globalSetup: './global-setup.ts',
  fullyParallel: true,
  workers: process.env.CI ? 4 : '50%',
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
    trace: 'retain-on-failure',
  },
})
