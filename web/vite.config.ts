import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// Tests see the same dates and hours on every machine: the test workers
// inherit this time zone, and src/test/setup.ts fixes the day.
process.env.TZ = 'Europe/Amsterdam'

export default defineConfig({
  plugins: [react()],
  server: { proxy: { '/api': 'http://localhost:8080' } },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.tsx'],
    // Test files share one environment per worker thread instead of building
    // a fresh one each: about a third less time. setup.ts resets what tests
    // change (DOM, handlers, storage, language, clock) after every test.
    pool: 'threads',
    isolate: false,
    // A test takes about a second; with every core busy one can stall for a
    // few. The limit only catches a hung test, it never slows a passing one.
    testTimeout: 10000,
  },
})
