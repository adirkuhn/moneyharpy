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
  },
})
