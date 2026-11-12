import '@testing-library/jest-dom/vitest'
import { cleanup, configure } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, vi } from 'vitest'
import i18n from '../i18n'
import { server } from './server'

/**
 * Every test runs on the same day: Wednesday 11 November 2026, noon in
 * Amsterdam (vite.config.ts pins the time zone). The clock still moves, from
 * there, so nothing waits on a frozen Date; a test that needs another moment
 * sets it before it renders.
 */
export const testNow = new Date('2026-11-11T11:00:00Z')

// Typing plain text key by key re-renders a form once per character, which
// made long forms the slowest tests. Plain text is pasted into the focused
// field instead: the same final value and change events, in one go. Text
// with special keys ({Enter}, [Shift>]...) is still typed key by key.
const typeKeyByKey = userEvent.type.bind(userEvent)
const typeFast: typeof userEvent.type = async (element, text, options) => {
  if (/[{[]/.test(text) || options) {
    return typeKeyByKey(element, text, options)
  }
  const user = userEvent.setup()
  await user.click(element)
  await user.paste(text)
}
Object.assign(userEvent, { type: typeFast })

// A loaded machine can take longer than the default second to render.
configure({ asyncUtilTimeout: 5000 })

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'], shouldAdvanceTime: true })
  vi.setSystemTime(testNow)
})
afterEach(async () => {
  cleanup()
  vi.useRealTimers()
  await i18n.changeLanguage('en')
  server.resetHandlers()
  localStorage.clear()
  document.documentElement.removeAttribute('data-theme')
})
afterAll(() => server.close())
