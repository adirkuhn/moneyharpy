import { expect, type Page, test } from '@playwright/test'

const password = 'correct horse battery'
const unique = () => `user${Date.now()}${Math.floor(Math.random() * 1e6)}@example.com`

async function register(page: Page, name = 'Ann de Vries', email = unique()) {
  await page.goto('/register')
  await page.getByLabel('Name').fill(name)
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Create account' }).click()
  await expect(page.getByRole('heading', { name: 'Select an administration' })).toBeVisible()
  return email
}

async function addAdministration(page: Page, name: string) {
  await page.goto('/administrations/new')
  await page.getByLabel('Company name').fill(name)
  await page.getByRole('button', { name: 'Add administration' }).click()
  await expect(page.getByRole('heading', { name })).toBeVisible()
}

test('visitors must sign in', async ({ page }) => {
  await page.goto('/administrations')
  await expect(page).toHaveURL(/\/login$/)
})

test('a user manages administrations and opens them by URI', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Harpy B.V.')
  const uri = new URL(page.url()).pathname
  expect(uri).toMatch(/^\/\d+$/)
  await addAdministration(page, 'Zeta Holding')

  await page.goto('/administrations')
  const rows = page.getByRole('list', { name: 'Administrations' }).getByRole('listitem')
  await expect(rows).toHaveCount(2)
  await expect(rows.first()).toContainText('Harpy B.V.')

  await page.getByRole('button', { name: 'Mark Zeta Holding as favorite' }).click()
  await expect(page.getByRole('button', { name: 'Remove Zeta Holding from favorites' })).toBeVisible()
  await page.reload()
  await expect(rows.first()).toContainText('Zeta Holding')

  await page.goto(uri)
  await expect(page.getByRole('heading', { name: 'Harpy B.V.' })).toBeVisible()
  await page.goto(`/r${uri}`)
  await expect(page).toHaveURL(new RegExp(`${uri}$`))
  await page.goto(`${uri}/settings`)
  await expect(page.getByRole('heading', { name: 'Harpy B.V. settings' })).toBeVisible()
})

test('another user cannot open an administration by URI', async ({ browser, page }) => {
  await register(page)
  await addAdministration(page, 'Private Co')
  const uri = new URL(page.url()).pathname

  const other = await (await browser.newContext()).newPage()
  await register(other, 'Bob')
  await other.goto(uri)
  await expect(other.getByRole('heading', { name: 'Page not found' })).toBeVisible()
})

test('a session survives reload and ends on sign out', async ({ page }) => {
  const email = await register(page)
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Select an administration' })).toBeVisible()

  await page.getByRole('button', { name: 'Ann de Vries' }).click()
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page).toHaveURL(/\/login$/)

  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill('not the password at all')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('alert')).toContainText('Invalid email or password')

  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Select an administration' })).toBeVisible()
})

test('an API token reads administrations in the Moneybird format', async ({ page, request }) => {
  await register(page)
  await addAdministration(page, 'API Co')

  await page.goto('/user')
  await page.getByLabel('Token name').fill('e2e')
  await page.getByRole('button', { name: 'Create token' }).click()
  const token = await page.locator('.secret code').innerText()

  const res = await request.get('/api/v2/administrations.json', { headers: { Authorization: `Bearer ${token}` } })
  expect(res.status()).toBe(200)
  const [row] = await res.json()
  expect(row).toMatchObject({ name: 'API Co', currency: 'EUR', access: 'user', suspended: false, period_locked_until: null })
  expect(typeof row.id).toBe('string')
})

test('the API documentation renders from the contract', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  // Redoc hard-codes its badge logo on a CDN. The CSP blocks it on purpose, so
  // the docs make no third-party requests.
  page.on('console', (m) => m.type() === 'error' && !m.text().includes('cdn.redoc.ly') && errors.push(m.text()))
  await page.goto('/docs/api/')
  await expect(page.getByRole('heading', { name: /MoneyHarpy API/ })).toBeVisible({ timeout: 20000 })
  await expect(page.getByText('/api/v2/administrations').first()).toBeVisible()
  expect(errors).toEqual([])
})
