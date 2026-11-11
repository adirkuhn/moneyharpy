import { createHmac } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, type Page, request as playwrightRequest, test } from '@playwright/test'

const password = 'correct horse battery'
const unique = () => `user${Date.now()}${Math.floor(Math.random() * 1e6)}@example.com`

const randomClient = () => ({
  'X-Forwarded-For': `10.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}`,
})
const mailpit = process.env.E2E_MAILPIT_URL ?? 'http://localhost:18025'
const admin = { email: 'admin@example.com', name: 'Instance Admin', password }

// Runs before every other test registers, so this account is the instance admin.
test.beforeAll(async ({ baseURL }) => {
  const api = await playwrightRequest.newContext({ baseURL, extraHTTPHeaders: randomClient() })
  const signedIn = await api.post('/api/auth/login', { data: { email: admin.email, password } })
  if (!signedIn.ok()) {
    expect((await api.post('/api/auth/register', { data: admin })).status()).toBe(201)
  }
  const saved = await api.patch('/api/instance/settings', {
    data: {
      smtp_enabled: true, smtp_host: 'mailpit', smtp_port: 1025, smtp_from: 'books@example.com', smtp_security: 'none',
      storage_enabled: true, storage_endpoint: 'http://storage:8333', storage_region: 'us-east-1', storage_bucket: 'moneyharpy',
      storage_access_key: 's3-test', storage_secret_key: 's3-test-secret', storage_path_style: true,
      webhooks_allow_local: true,
    },
  })
  expect(saved.status()).toBe(200)
  // Creates the bucket on first run.
  expect((await api.post('/api/instance/settings/test_storage')).status()).toBe(204)
  await api.dispose()
})

async function resetLinkFor(email: string): Promise<string> {
  return linkFor(email, /https?:\/\/\S+\/passwords\/edit\?token=\S+/)
}

async function linkFor(email: string, pattern: RegExp): Promise<string> {
  const api = await playwrightRequest.newContext()
  let link = ''
  await expect
    .poll(async () => {
      const found = await (await api.get(`${mailpit}/api/v1/search?query=${encodeURIComponent(`to:${email}`)}`)).json()
      if (found.messages.length === 0) {
        return false
      }
      const message = await (await api.get(`${mailpit}/api/v1/message/${found.messages[0].ID}`)).json()
      link = pattern.exec(message.Text)?.[0] ?? ''
      return link !== ''
    }, { timeout: 15000 })
    .toBe(true)
  await api.dispose()
  return link
}

async function register(page: Page, name = 'Ann de Vries', email = unique()) {
  await page.setExtraHTTPHeaders(randomClient())
  await page.goto('/register')
  await page.getByLabel('Name').fill(name)
  await page.getByLabel('Email address').fill(email)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Create account' }).click()
  await expect(page.getByRole('heading', { name: 'Select an administration' })).toBeVisible()
  return email
}

async function addAdministration(page: Page, name: string) {
  await page.goto('/onboarding/company/manually?country=NL')
  await page.getByLabel('Company name').fill(name)
  await page.getByRole('button', { name: 'Next' }).click()
  await expect(page.getByRole('heading', { name: 'What do you want to start with?' })).toBeVisible()
  await page.getByRole('radio', { name: /open my administration/ }).check()
  await page.getByRole('button', { name: 'Get started' }).click()
  await expect(page).toHaveURL(/\/\d+\/feed$/)
  await expect(page.getByRole('button', { name, exact: true })).toBeVisible()
}

/** The open administration's URI, such as /123, from its overview at /123/feed. */
function administrationUri(page: Page) {
  return new URL(page.url()).pathname.replace(/\/feed$/, '')
}

test('visitors must sign in', async ({ page }) => {
  await page.goto('/administrations')
  await expect(page).toHaveURL(/\/login$/)
})

test('a user manages administrations and opens them by URI', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Harpy B.V.')
  const uri = administrationUri(page)
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
  await expect(page).toHaveURL(new RegExp(`${uri}/feed$`))
  await expect(page.getByRole('heading', { name: /^Good (morning|afternoon|evening) Ann!$/ })).toBeVisible()
  await page.getByRole('button', { name: 'Harpy B.V.' }).click()
  const switcher = page.getByRole('navigation', { name: 'Your administrations' })
  await expect(switcher.getByRole('link', { name: 'Harpy B.V.' })).toHaveAttribute('aria-current', 'true')
  await expect(switcher.getByRole('link', { name: 'Zeta Holding' })).not.toHaveAttribute('aria-current')
  await page.keyboard.press('Escape')
  await page.goto(`/r${uri}`)
  await expect(page).toHaveURL(new RegExp(`${uri}/feed$`))
  await page.goto(`${uri}/settings`)
  await expect(page).toHaveURL(new RegExp(`${uri}/settings/company$`))
  await expect(page.getByRole('heading', { name: 'Company', exact: true })).toBeVisible()
})

test('adding an administration walks through the wizard', async ({ page }) => {
  await register(page)
  await page.getByRole('link', { name: 'Add administration' }).click()
  await expect(page.getByRole('heading', { name: 'Welcome Ann' })).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Main' })).toHaveCount(0)
  await page.getByRole('link', { name: 'Next' }).click()

  await page.getByLabel('Company name').fill('Wizard Co')
  await page.getByLabel('Address', { exact: true }).fill('Dam 1')
  await page.getByLabel('Zipcode').fill('1012JS')
  await page.getByLabel('City').fill('Amsterdam')
  await page.getByRole('combobox', { name: 'Currency' }).click()
  await page.getByRole('searchbox', { name: 'Find currency' }).fill('USD')
  await page.getByRole('option', { name: /^USD/ }).click()
  await page.getByRole('button', { name: 'Next' }).click()

  await expect(page).toHaveURL(/\/\d+\/onboarding\/first_action\/new/)
  await page.getByRole('radio', { name: /send an invoice/ }).check()
  await page.getByRole('button', { name: 'Get started' }).click()
  await expect(page.getByRole('button', { name: 'Wizard Co', exact: true })).toBeVisible()

  const id = administrationUri(page).slice(1)
  const administration = await (await page.request.get(`/api/v2/administrations/${id}`)).json()
  expect(administration).toMatchObject({ currency: 'USD', country: 'NL' })
  const company = await (await page.request.get(`/api/v2/${id}/identities/default.json`)).json()
  expect(company).toMatchObject({ company_name: 'Wizard Co', address1: 'Dam 1', zipcode: '1012JS', city: 'Amsterdam', country: 'NL' })
})

test('company details and sender addresses are managed in settings', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Settings Co')
  const uri = administrationUri(page)

  await page.goto('/administrations')
  await page.getByRole('link', { name: 'Settings for Settings Co' }).click()
  await expect(page).toHaveURL(new RegExp(`${uri}/settings/company$`))
  const details = page.getByRole('region', { name: 'Company details' })
  await expect(details).toContainText('Settings Co')

  await details.getByRole('link', { name: 'Edit' }).click()
  await expect(page.getByRole('heading', { name: 'Edit company information' })).toBeVisible()
  await page.getByLabel('Phone number').fill('0201234567')
  const account = page.getByLabel('Account number')
  await account.fill('NL91 ABNA 0417 1643 01')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(account).toHaveAccessibleDescription(/^Account number is not a valid IBAN\. Check it for typos/)
  await expect(page.getByText(/Account number is not a valid IBAN/)).toBeVisible()
  await account.fill('NL91 ABNA 0417 1643 00')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(details).toContainText('0201234567')
  await expect(details).toContainText('NL91 ABNA 0417 1643 00')

  await details.getByRole('link', { name: 'Edit' }).click()
  await page.getByRole('link', { name: 'Add custom field' }).click()
  await expect(page.getByRole('heading', { name: 'New custom field' })).toBeVisible()
  await page.getByLabel('Name').fill('Slogan')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('heading', { name: 'Edit company information' })).toBeVisible()
  await page.getByLabel('Slogan').fill('Fresh bread')
  await page.getByRole('checkbox', { name: 'Company has been registered for the small business scheme (KOR)' }).check()
  await page.getByLabel('Website', { exact: true }).fill('https://settings.example')
  await page.getByRole('checkbox', { name: 'Company does not have a VAT number' }).check()
  await page.getByLabel('Reason').selectOption('industry_exemption')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page).toHaveURL(new RegExp(`${uri}/settings/company$`))
  const id = uri.slice(1)
  const saved = await (await page.request.get(`/api/v2/${id}/identities/default.json`)).json()
  expect(saved.custom_fields).toEqual([expect.objectContaining({ name: 'Slogan', value: 'Fresh bread' })])
  expect(saved.tax_number_absence_reason).toBe('industry_exemption')
  const profile = await (await page.request.get(`/api/v2/administrations/${id}.json`)).json()
  expect(profile).toMatchObject({ small_business_scheme: true, company_website_url: 'https://settings.example' })

  const senders = page.getByRole('region', { name: 'Sender addresses' })
  await senders.getByRole('link', { name: 'Add' }).click()
  await page.getByLabel('Company name').fill('Side Trade')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(senders).toContainText('Side Trade')

  await senders.getByRole('link', { name: 'Edit Side Trade' }).click()
  const editUri = new URL(page.url()).pathname
  await page.goto(editUri.replace(/\/edit$/, ''))
  await expect(page).toHaveURL(new RegExp(`${editUri}$`))
  await page.getByRole('button', { name: 'Delete' }).click()
  await page.getByRole('button', { name: 'Confirm delete' }).click()
  await expect(senders).not.toContainText('Side Trade')

  await page.getByRole('navigation', { name: 'Settings' }).getByRole('link', { name: 'Management' }).click()
  await page.getByLabel('Name').fill('Renamed Co')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('status')).toContainText('Saved')
  await expect(page.getByRole('button', { name: 'Renamed Co' })).toBeVisible()
})

test('the whole app switches to Dutch and remembers it', async ({ page }) => {
  await register(page)
  await page.getByRole('button', { name: 'Ann de Vries' }).click()
  await page.getByLabel('Language').selectOption('nl')
  await expect(page.getByRole('heading', { name: 'Kies een administratie' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Kies een administratie' })).toBeVisible()
})

test('a forgotten password can be replaced through email', async ({ page }) => {
  const email = await register(page)
  await page.getByRole('button', { name: 'Ann de Vries' }).click()
  await page.getByRole('button', { name: 'Sign out' }).click()

  await page.getByRole('link', { name: 'Forgot your password?' }).click()
  await expect(page.getByRole('heading', { name: 'Forgot your password?' })).toBeVisible()
  await page.getByLabel('Email address').fill(email)
  await page.getByRole('button', { name: 'Send reset link' }).click()
  await expect(page.getByRole('status')).toContainText('we sent a link')

  const link = new URL(await resetLinkFor(email))
  await page.goto(link.pathname + link.search)
  await page.getByLabel('New password').fill('a brand new password')
  await page.getByRole('button', { name: 'Save new password' }).click()
  await expect(page.getByRole('status')).toContainText('Password changed')
  await expect(page.getByRole('heading', { name: 'Login', exact: true })).toBeVisible()

  await page.getByLabel('Email address').fill(email)
  await page.getByLabel('Password').fill('a brand new password')
  await page.getByRole('button', { name: 'Login to MoneyHarpy' }).click()
  await expect(page.getByRole('heading', { name: 'Select an administration' })).toBeVisible()

  await page.goto(link.pathname + link.search)
  await page.getByLabel('New password').fill('yet another password')
  await page.getByRole('button', { name: 'Save new password' }).click()
  await expect(page.getByRole('alert')).toContainText('invalid or has expired')
})

test('another user cannot open an administration by URI', async ({ browser, page }) => {
  await register(page)
  await addAdministration(page, 'Private Co')
  const uri = administrationUri(page)

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

  await page.getByLabel('Email address').fill(email)
  await page.getByLabel('Password').fill('not the password at all')
  await page.getByRole('button', { name: 'Login to MoneyHarpy' }).click()
  await expect(page.getByRole('alert')).toContainText('Invalid email or password')

  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Login to MoneyHarpy' }).click()
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

// A 1x1 transparent PNG.
const tinyPNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==', 'base64')

test('invoices and estimates are set up in settings', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Invoice Co')
  const uri = administrationUri(page)
  await page.goto(`${uri}/settings/company`)
  await page.getByRole('navigation', { name: 'Settings' }).getByRole('link', { name: 'Invoices and estimates' }).click()
  await expect(page.getByRole('heading', { name: 'Invoices and estimates', level: 1 })).toBeVisible()

  const numbering = page.getByRole('group', { name: 'Invoice number' })
  await numbering.getByLabel('Template').fill('F{year}-{id}')
  await numbering.getByLabel('Next number').fill('0100')
  await numbering.getByRole('button', { name: 'Save' }).click()
  await expect(numbering.getByText(`Example: F${new Date().getFullYear()}-0100`)).toBeVisible()
  await expect(numbering.getByRole('button', { name: 'Save' })).toBeHidden()

  await page.getByRole('region', { name: 'Workflows' }).getByRole('button', { name: 'Add' }).click()
  await page.getByRole('link', { name: 'Add invoice workflow' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Export')
  await page.getByRole('button', { name: 'Add reminder' }).click()
  await page.getByRole('group', { name: 'Reminder 1' }).getByLabel('Reminder text').fill('Please pay')
  await page.getByRole('button', { name: 'Save' }).click()
  const invoiceFlows = page.getByRole('table', { name: 'Invoice workflows' })
  await expect(invoiceFlows.getByRole('link', { name: 'Export' })).toBeVisible()
  await expect(invoiceFlows.getByRole('button', { name: 'Default workflow cannot be deleted' })).toBeDisabled()

  const styles = page.getByRole('region', { name: 'Document styles' })
  await styles.getByRole('link', { name: 'Add' }).click()
  await page.getByLabel('Name', { exact: true }).fill('The Best')
  await page.getByRole('button', { name: 'Save' }).click()
  await styles.getByRole('link', { name: 'The Best' }).click()
  await page.getByLabel('Upload Logo').setInputFiles({ name: 'logo.png', mimeType: 'image/png', buffer: tinyPNG })
  const logo = page.getByRole('img', { name: 'Current Logo' })
  await expect(logo).toBeVisible()
  await expect.poll(() => logo.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1)

  await page.goto(`${uri}/settings/invoices_and_estimates`)
  await page.getByRole('region', { name: 'Custom fields' }).getByRole('link', { name: 'Add' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Slogan')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('table', { name: 'Sender address' })).toContainText('Slogan')

  const id = uri.slice(1)
  const workflows = await (await page.request.get(`/api/v2/${id}/workflows.json`)).json()
  expect(workflows.map((w: { name: string }) => w.name)).toContain('Export')
})

test('bookkeeping starts from the Dutch standard and is adjusted in settings', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Books Co')
  const uri = administrationUri(page)
  await page.goto(`${uri}/settings/company`)
  await page.getByRole('navigation', { name: 'Settings' }).getByRole('link', { name: 'Bookkeeping' }).click()
  await expect(page.getByRole('heading', { name: 'Bookkeeping', level: 1 })).toBeVisible()

  const vat = page.getByRole('group', { name: 'VAT return' })
  await vat.getByText('Month').click()
  await page.getByRole('button', { name: 'Change' }).click()
  await expect(vat.getByRole('radio', { name: 'Month' })).toBeChecked()

  const sales = page.getByRole('table', { name: 'Sales' })
  await expect(sales.getByText('VAT 21%').first()).toBeVisible()
  await page.getByRole('region', { name: 'VAT rates' }).getByRole('link', { name: 'Add VAT rate' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Reduced 9.5%')
  await page.getByLabel('Percentage').fill('9,5')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('table', { name: 'Sales' }).getByRole('link', { name: 'Reduced 9.5%' })).toBeVisible()

  const categories = page.getByRole('region', { name: 'Categories' })
  await categories.getByRole('button', { name: 'Add' }).click()
  await page.getByRole('link', { name: 'Add category' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Hosting')
  await page.getByLabel('Account number').fill('4460')
  await page.getByLabel('Part of').selectOption({ label: '4400 Office costs' })
  await page.getByLabel('Chart scheme code').fill('WBedKanSof')
  await page.getByRole('button', { name: 'Save' }).click()
  const pl = page.getByRole('table', { name: 'Profit and loss' })
  await expect(pl.getByRole('link', { name: 'Hosting' })).toBeVisible()

  await categories.getByRole('button', { name: 'Add' }).click()
  await page.getByRole('link', { name: 'Add from template' }).click()
  await page.getByRole('button', { name: 'Add Company car' }).click()
  await expect(page.getByRole('region', { name: 'Company car' }).getByText('Added')).toBeVisible()

  const id = uri.slice(1)
  const accounts = await (await page.request.get(`/api/v2/${id}/ledger_accounts.json`)).json()
  const hosting = accounts.find((a: { name: string }) => a.name === 'Hosting')
  expect(hosting.taxonomy_item.code).toBe('WBedKanSof')
  expect(accounts.map((a: { name: string }) => a.name)).toContain('Company car fuel')
})

test('bank statements are imported, booked by rules and by hand', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Bank Co')
  const uri = administrationUri(page)
  await page.goto(`${uri}/settings/bank`)
  await expect(page.getByRole('heading', { name: 'Bank', level: 1 })).toBeVisible()
  await expect(page.getByRole('table', { name: 'Financial accounts' }).getByRole('link', { name: 'Cash' })).toBeVisible()

  const optionValue = async (label: string, text: string) =>
    (await page.getByLabel(label, { exact: true }).locator('option', { hasText: text }).first().getAttribute('value')) ?? ''

  await page.getByRole('region', { name: 'Booking rules' }).getByRole('link', { name: 'Add' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Hosting')
  await page.getByLabel('Value').fill('hosting')
  await page.getByLabel('Category').selectOption(await optionValue('Category', 'Software'))
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('table', { name: 'Booking rules' }).getByRole('link', { name: 'Hosting' })).toBeVisible()

  await page.getByRole('region', { name: 'Accounts' }).getByRole('link', { name: 'Add' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Main bank')
  await page.getByLabel('Account number').fill('NL91 ABNA 0417 1643 00')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('heading', { name: 'Main bank', level: 1 })).toBeVisible()
  // The example statement, dated today so it falls in this year's transactions.
  const today = new Date().toISOString().slice(0, 10)
  const camt = readFileSync(join(__dirname, '../../internal/bankfile/testdata/camt053.xml'), 'utf8').replace(/2026-1[01]-\d\d/g, today)
  await page.getByLabel('Choose file').setInputFiles({ name: 'statement.xml', mimeType: 'text/xml', buffer: Buffer.from(camt) })
  await expect(page.getByRole('status')).toContainText('2 new transactions.')
  await expect(page.getByRole('status')).toContainText('1 booked by a rule.')

  await page.getByRole('link', { name: 'Back' }).click()
  const menu = page.getByRole('navigation', { name: 'Administration menu' })
  await menu.getByRole('button', { name: 'Bank' }).click()
  await menu.getByRole('link', { name: 'Transactions' }).click()
  await page.getByRole('button', { name: 'Transactions to book' }).click()
  await page.getByRole('search', { name: 'Filter transactions' }).getByRole('link', { name: 'All transactions' }).click()
  const table = page.getByRole('table', { name: 'Transactions' })
  await expect(table.getByRole('link', { name: 'Hosting Company' })).toBeVisible()
  await table.getByRole('link', { name: 'Example Client B.V.' }).click()

  const form = page.getByRole('form', { name: 'Book' })
  await form.getByLabel('Category').selectOption(await optionValue('Category', 'Revenue'))
  await form.getByLabel('Amount').fill('600')
  await form.getByRole('button', { name: 'Book' }).click()
  await expect(page.getByRole('table', { name: 'Bookings' }).getByText('Revenue')).toBeVisible()
  await page.getByRole('form', { name: 'Book' }).getByLabel('Category').selectOption(await optionValue('Category', 'Revenue'))
  await page.getByRole('form', { name: 'Book' }).getByRole('button', { name: 'Book' }).click()
  await expect(page.getByRole('form', { name: 'Book' })).toBeHidden()

  const id = uri.slice(1)
  const balance = await (await page.request.get(`/api/v2/${id}/reports/bank_balance`)).json()
  expect(balance.total).toBe('974.5')

  // The overview sums it up, and the logo leads back to it from anywhere.
  await page.getByRole('link', { name: 'MoneyHarpy home' }).click()
  await expect(page).toHaveURL(new RegExp(`${uri}/feed$`))
  const bank = page.getByRole('region', { name: 'Bank' })
  await expect(bank.getByRole('link', { name: /Balance\s*€974\.50/ })).toBeVisible()
  await expect(bank.getByRole('link', { name: /To link\s*0/ })).toBeVisible()
  const recent = page.getByRole('region', { name: 'Recently processed' })
  await expect(recent.getByRole('link')).toHaveCount(2)
  await expect(recent.getByRole('link', { name: /Example Client B\.V\./ })).toContainText('Booked')
  await expect(page.getByRole('region', { name: 'Incoming' })).toContainText('To pay — 0')

  await page.goto('/user')
  await page.getByRole('link', { name: 'MoneyHarpy home' }).click()
  await expect(page).toHaveURL(new RegExp(`${uri}/feed$`))
  await page.goto('/')
  await expect(page).toHaveURL(new RegExp(`${uri}/feed$`))
})


// totp computes an authenticator code (RFC 6238) from a base32 secret.
function totp(secret: string, at = Date.now()): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret) {
    bits += alphabet.indexOf(c).toString(2).padStart(5, '0')
  }
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 30000)))
  const mac = createHmac('sha1', key).update(counter).digest()
  const o = mac[mac.length - 1] & 0x0f
  return String((mac.readUInt32BE(o) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

test('owners invite users with limited rights and require two-step verification', async ({ browser, page }) => {
  const ownerEmail = await register(page)
  await addAdministration(page, 'Team Co')
  const uri = administrationUri(page)
  await page.goto(`${uri}/settings/administration`)
  await expect(page.getByRole('heading', { name: 'Management', level: 1 })).toBeVisible()

  const colleagueEmail = unique()
  await page.getByRole('region', { name: 'Users' }).getByRole('button', { name: 'Invite user' }).click()
  await page.getByRole('link', { name: 'Employee' }).click()
  await page.getByLabel('Email address').fill(colleagueEmail)
  for (const name of ['Invoices', 'Purchase invoices and receipts', 'Quotes', 'Time tracking']) {
    await page.getByRole('checkbox', { name, exact: true }).uncheck()
  }
  await page.getByRole('checkbox', { name: 'Bank', exact: true }).check()
  await page.getByRole('button', { name: 'Send invitation' }).click()
  await expect(page.getByRole('table', { name: 'Open invitations' })).toContainText(colleagueEmail)

  const link = new URL(await linkFor(colleagueEmail, /https?:\/\/\S+\/invitations\/\S+/))
  const colleague = await (await browser.newContext()).newPage()
  await colleague.setExtraHTTPHeaders(randomClient())
  await colleague.goto(link.pathname)
  await expect(colleague.getByRole('heading', { name: 'Join Team Co' })).toBeVisible()
  await colleague.getByRole('link', { name: 'Create an account' }).click()
  await colleague.getByLabel('Name').fill('Colleague')
  await colleague.getByLabel('Password').fill(password)
  await colleague.getByRole('button', { name: 'Create account' }).click()
  await colleague.getByRole('button', { name: 'Accept invitation' }).click()
  await expect(colleague).toHaveURL(new RegExp(`${uri}/feed$`))
  await expect(colleague.getByRole('navigation', { name: 'Main' }).getByRole('button', { name: 'Team Co' })).toBeVisible()
  await expect(colleague.getByRole('navigation', { name: 'Administration menu' }).getByRole('button', { name: 'Bank' })).toBeVisible()
  await colleague.goto(`${uri}/settings/bookkeeping`)
  await expect(colleague.getByRole('heading', { name: 'No access' })).toBeVisible()
  const api = await colleague.request.get(`/api/v2${uri}/bookkeeping_settings`)
  expect(api.status()).toBe(403)

  // The owner turns on two-step verification and requires it.
  await page.goto('/user')
  await page.getByRole('button', { name: 'Set up two-step verification' }).click()
  const key = (await page.locator('.secret-inline').textContent()) ?? ''
  await page.getByLabel('Code').fill(totp(key))
  await page.getByRole('button', { name: 'Turn on' }).click()
  await expect(page.getByText('Save these recovery codes')).toBeVisible()
  const recovery = (await page.locator('.recovery-codes code').first().textContent()) ?? ''

  await page.goto(`${uri}/settings/administration#security`)
  await page.getByRole('switch', { name: 'Require two-step verification' }).click({ force: true })
  await page.getByRole('button', { name: 'Require two-step verification' }).click()
  await expect(page.getByText('This cannot be turned off again.').first()).toBeVisible()

  await colleague.goto(`${uri}/financial_mutations`)
  await expect(colleague.getByText('Team Co requires two-step verification.')).toBeVisible()

  // Signing in now takes the second step.
  await page.getByRole('button', { name: 'Ann de Vries' }).click()
  await page.getByRole('button', { name: 'Sign out' }).click()
  await page.goto('/login')
  await page.getByLabel('Email address').fill(ownerEmail)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Login to MoneyHarpy' }).click()
  await page.getByLabel('Code').fill(recovery)
  await page.getByRole('button', { name: 'Verify' }).click()
  await expect(page.getByRole('heading', { name: 'Select an administration' })).toBeVisible()

  await page.goto(`${uri}/delete`)
  await page.getByLabel('Administration name').fill('Team Co')
  await page.getByRole('button', { name: 'Delete permanently' }).click()
  await expect(page.getByRole('heading', { name: 'Select an administration' })).toBeVisible()
  await expect(page.getByText('Team Co')).toHaveCount(0)
})


test('external applications, AI assistants and webhooks connect to one administration', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Connect Co')
  const uri = administrationUri(page)
  const api = `/api/v2${uri}`
  await page.goto(`${uri}/settings/connections`)
  await expect(page.getByRole('heading', { name: 'Integrations', level: 1 })).toBeVisible()
  await expect(page.getByText('Coming later')).toHaveCount(4)

  // A read-only API token with only the bank scope.
  await page.getByRole('link', { name: 'New API token' }).click()
  await page.getByLabel('Name', { exact: true }).fill('Shop')
  await page.getByLabel('Access').selectOption('read')
  await page.getByRole('checkbox', { name: 'Bank', exact: true }).check()
  await page.getByRole('button', { name: 'Create' }).click()
  const token = (await page.getByRole('status').locator('code').textContent()) ?? ''
  expect(token).toMatch(/^mhc_/)
  const app = await playwrightRequest.newContext({ baseURL: new URL(page.url()).origin, extraHTTPHeaders: { Authorization: `Bearer ${token}`, ...randomClient() } })
  expect((await app.get(`${api}/financial_mutations`)).status()).toBe(200)
  expect((await app.get(`${api}/tax_rates`)).status()).toBe(403)
  expect((await app.post(`${api}/financial_accounts`, { data: { financial_account: { name: 'Nope' } } })).status()).toBe(403)
  await app.dispose()

  // An AI connection speaks MCP.
  await page.goto(`${uri}/connections/new?kind=ai`)
  await page.getByLabel('Name', { exact: true }).fill('Assistant')
  await page.getByRole('checkbox', { name: 'Settings', exact: true }).check()
  await page.getByRole('button', { name: 'Create' }).click()
  const aiToken = (await page.getByRole('status').locator('code').textContent()) ?? ''
  const assistant = await playwrightRequest.newContext({ baseURL: new URL(page.url()).origin, extraHTTPHeaders: { Authorization: `Bearer ${aiToken}`, ...randomClient() } })
  const listed = await (await assistant.post('/api/mcp', { data: { jsonrpc: '2.0', id: 1, method: 'tools/list' } })).json()
  const names = listed.result.tools.map((t: { name: string }) => t.name)
  expect(names).toContain('listTaxRates')
  const called = await (await assistant.post('/api/mcp', { data: { jsonrpc: '2.0', id: 2, method: 'tools/call', params: { name: 'listTaxRates', arguments: {} } } })).json()
  expect(called.result.isError).toBe(false)
  expect(called.result.content[0].text).toContain('"tax_rate_type":"sales_invoice"')
  await assistant.dispose()

  // A webhook receives a signed notification when a VAT rate is added.
  await page.goto(`${uri}/webhooks/new`)
  await page.getByLabel('URL').fill('http://app:8080/healthz')
  await page.getByRole('checkbox', { name: 'Everything about VAT rates' }).check()
  await page.getByRole('button', { name: 'Create webhook' }).click()
  await expect(page.getByText('Copy this signing secret now.')).toBeVisible()
  expect((await page.request.post(`${api}/tax_rates`, { data: { tax_rate: { name: 'Hooked 7%', percentage: '7', tax_rate_type: 'sales_invoice' } } })).status()).toBe(201)
  await expect.poll(async () => {
    const hooks = await (await page.request.get(`${api}/webhooks`)).json()
    return hooks[0]?.last_http_status
  }, { timeout: 20000 }).toBe(200)
  await page.goto(`${uri}/settings/connections`)
  await expect(page.getByRole('table', { name: 'Webhooks' })).toContainText('http://app:8080/healthz')
  await expect(page.getByRole('table', { name: 'External applications' })).toContainText('Shop')
})

test('contacts are added, searched, completed with people and notes, and archived', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Contacts Co')
  const menu = page.getByRole('navigation', { name: 'Administration menu' })
  await menu.getByRole('link', { name: 'Contacts' }).click()
  await expect(page.getByText('No contacts yet. Add your first customer or supplier.')).toBeVisible()

  await page.getByRole('link', { name: 'New contact' }).click()
  await page.getByLabel('Company name').fill('Example Shop B.V.')
  await page.getByLabel('City').fill('Utrecht')
  await page.getByLabel('Email for invoices').fill('billing@example.com')
  await page.getByRole('checkbox', { name: 'Collect by direct debit (SEPA mandate)' }).check()
  await page.getByLabel('IBAN').fill('NL91ABNA0417164300')
  await page.getByLabel('Account holder').fill('Example Shop')
  await page.getByLabel('Mandate reference').fill('MANDATE-1')
  await page.getByLabel('Mandate date').fill('2026-11-01')
  await page.getByRole('button', { name: 'Save contact' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Example Shop B.V.' })).toBeVisible()
  await expect(page.getByText('Customer 1')).toBeVisible()
  await expect(page.getByRole('region', { name: 'Contact details' })).toContainText('billing@example.com')

  const people = page.getByRole('region', { name: 'Contact people' })
  await people.getByRole('button', { name: 'Add contact person' }).click()
  await people.getByLabel('First name').fill('Zoë')
  await people.getByLabel('Email').fill('zoe@example.com')
  await people.getByRole('button', { name: 'Save contact person' }).click()
  await expect(people.getByText('Zoë')).toBeVisible()

  const notes = page.getByRole('region', { name: 'Notes' })
  await notes.getByLabel('New note').fill('Send the contract')
  await notes.getByRole('checkbox', { name: 'This is a to-do' }).check()
  await notes.getByRole('button', { name: 'Add note' }).click()
  await expect(notes.getByText('Send the contract')).toBeVisible()

  await page.getByRole('link', { name: 'To Contacts' }).click()
  await page.getByRole('link', { name: 'New contact' }).click()
  await page.getByLabel('First name').fill('Ann')
  await page.getByLabel('Last name').fill('Jansen')
  await page.getByRole('button', { name: 'Save contact' }).click()
  await expect(page.getByText('Customer 2')).toBeVisible()
  await page.getByRole('button', { name: 'More actions' }).click()
  await page.getByRole('button', { name: 'Archive' }).click()
  await expect(page.getByText('This contact is archived. It no longer shows in lists.')).toBeVisible()

  await page.getByRole('link', { name: 'To Contacts' }).click()
  const table = page.getByRole('table', { name: 'Contacts' })
  await expect(table.getByRole('row')).toHaveCount(2)
  await page.getByRole('searchbox', { name: 'Search contacts' }).fill('zoe@')
  await expect(table.getByRole('link', { name: 'Example Shop B.V.' })).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search contacts' }).fill('')
  await page.getByRole('button', { name: 'All contacts' }).click()
  const filters = page.getByRole('search', { name: 'Contact filters' })
  await filters.getByRole('checkbox', { name: 'Show archived contacts' }).check()
  await filters.getByRole('button', { name: 'Filter' }).click()
  await expect(page.getByRole('button', { name: 'All contacts, archived included' })).toBeVisible()
  await expect(table.getByRole('link', { name: 'Ann Jansen' })).toBeVisible()
})

test('a journal entry made by hand shows in the trial balance', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Journal Co')
  const menu = page.getByRole('navigation', { name: 'Administration menu' })
  await menu.getByRole('button', { name: 'Reports' }).click()
  await menu.getByRole('link', { name: 'Journal entries' }).click()
  await expect(page.getByText('No journal entries in this period.')).toBeVisible()

  await page.getByRole('link', { name: 'New journal entry' }).click()
  await page.getByLabel('Reference').fill('Rent paid privately')
  const rows = page.getByRole('table', { name: 'Rows' }).getByRole('row')
  const optionValue = async (row: number, text: string) =>
    (await rows.nth(row).getByLabel('Category').locator('option', { hasText: text }).first().getAttribute('value')) ?? ''
  await rows.nth(1).getByLabel('Category').selectOption(await optionValue(1, 'Rent'))
  await rows.nth(1).getByLabel('Debit').fill('400')
  await rows.nth(2).getByLabel('Category').selectOption(await optionValue(2, 'Private deposits'))
  await rows.nth(2).getByLabel('Credit').fill('400')
  await expect(page.getByText('Debit and credit are equal.')).toBeVisible()
  await page.getByRole('button', { name: 'Save journal entry' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Rent paid privately' })).toBeVisible()

  await menu.getByRole('link', { name: 'Trial balance' }).click()
  const table = page.getByRole('table', { name: 'Trial balance' })
  await expect(table.getByRole('row', { name: /Rent/ })).toContainText('€400.00')
  await expect(table.getByRole('row', { name: /Total/ })).toContainText('€400.00')
  // The period button above the report switches to another period.
  await page.getByRole('button', { name: 'This month' }).click()
  await page.getByRole('search', { name: 'Report filters' }).getByRole('link', { name: 'This year' }).click()
  await expect(page).toHaveURL(/period=this_year/)
  await expect(table.getByRole('row', { name: /Rent/ })).toContainText('€400.00')
  await table.getByRole('link', { name: /Rent/ }).click()
  await expect(page.getByRole('button', { name: 'This year' })).toBeVisible()
  await page.getByRole('table', { name: 'Bookings' }).getByRole('link', { name: 'Journal entry' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Rent paid privately' })).toBeVisible()
})

test('a sales invoice is written, sent by email, downloaded and credited', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Invoice Co')
  const uri = administrationUri(page)
  const customer = unique()
  const created = await page.request.post(`/api/v2${uri}/contacts`, { data: { contact: { company_name: 'Example Shop B.V.', send_invoices_to_email: customer } } })
  expect(created.status()).toBe(201)

  const menu = page.getByRole('navigation', { name: 'Administration menu' })
  await menu.getByRole('button', { name: 'Invoices' }).click()
  await menu.getByRole('link', { name: 'Sales invoices' }).click()
  await expect(page.getByText('No invoices yet. Create your first invoice.')).toBeVisible()

  await page.getByRole('link', { name: 'New invoice' }).click()
  await page.getByRole('combobox', { name: 'Contact' }).click()
  await page.getByRole('searchbox', { name: 'Find a contact' }).fill('Example')
  await page.getByRole('option', { name: 'Example Shop B.V.' }).click()
  await page.getByLabel('Reference').fill('Order 12')
  const rows = page.getByRole('table', { name: 'Lines' }).getByRole('row')
  const vat21 = async (row: number) =>
    (await rows.nth(row).getByLabel('VAT rate').locator('option', { hasText: '21%' }).first().getAttribute('value')) ?? ''
  await rows.nth(1).getByLabel('Description').fill('Consulting')
  await rows.nth(1).getByLabel('Quantity').fill('2')
  await rows.nth(1).getByLabel('Price').fill('100')
  await rows.nth(1).getByLabel('VAT rate').selectOption(await vat21(1))
  await page.getByRole('button', { name: 'Add line' }).click()
  await rows.nth(2).getByLabel('Description').fill('Travel costs')
  await rows.nth(2).getByLabel('Price').fill('50')
  await rows.nth(2).getByLabel('VAT rate').selectOption(await vat21(2))
  const totals = page.getByRole('table', { name: 'Totals' })
  await expect(totals.getByRole('row', { name: /^Total/ })).toContainText('€302.50')
  await page.getByRole('button', { name: 'Save invoice' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Draft #1' })).toBeVisible()

  await page.getByRole('button', { name: 'Send', exact: true }).click()
  const sending = page.getByRole('region', { name: 'Send invoice' })
  await expect(sending.getByLabel('Email address')).toHaveValue(customer)
  await sending.getByRole('button', { name: 'Send invoice' }).click()
  await expect(page.locator('.badge', { hasText: 'Open' })).toBeVisible()
  await expect(page.getByRole('heading', { level: 1 })).not.toHaveText(/Draft/)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/\d/)
  await expect(page.getByRole('table', { name: 'Totals' }).getByRole('row', { name: /^Total/ })).toContainText('€302.50')

  const mail = await playwrightRequest.newContext()
  await expect
    .poll(async () => (await (await mail.get(`${mailpit}/api/v1/search?query=${encodeURIComponent(`to:${customer}`)}`)).json()).messages.length, { timeout: 15000 })
    .toBe(1)
  await mail.dispose()

  const pdf = await page.request.get((await page.getByRole('link', { name: 'Download PDF' }).getAttribute('href')) ?? '')
  expect(pdf.status()).toBe(200)
  expect(pdf.headers()['content-type']).toContain('application/pdf')

  const attachments = page.getByRole('region', { name: 'Attachments' })
  await attachments.getByLabel('Add attachment').setInputFiles({ name: 'order.txt', mimeType: 'text/plain', buffer: Buffer.from('Order 12') })
  await expect(attachments.getByRole('link', { name: 'order.txt' })).toBeVisible()

  await page.getByRole('button', { name: 'More actions' }).click()
  await page.getByRole('button', { name: 'Create credit invoice' }).click()
  await expect(page.getByRole('heading', { level: 1, name: /^Draft/ })).toBeVisible()
  await expect(page.locator('.badge', { hasText: 'Draft' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'the original invoice' })).toBeVisible()
  await expect(page.getByRole('table', { name: 'Totals' }).getByRole('row', { name: /^Total/ })).toContainText('-€302.50')

  await page.getByRole('link', { name: 'To Invoices' }).click()
  await page.getByRole('button', { name: 'Invoices from this year' }).click()
  await page.getByRole('search', { name: 'Invoice filters' }).getByRole('link', { name: 'Unpaid invoices' }).click()
  await expect(page.getByRole('button', { name: 'Unpaid invoices' })).toBeVisible()
  await expect(page.getByRole('table', { name: 'Invoices' }).getByRole('row')).toHaveCount(2)
  // Ticking a row brings up the bar of actions on the ticked invoices.
  const tick = page.getByRole('table', { name: 'Invoices' }).getByRole('row').nth(1).getByRole('checkbox')
  await tick.check()
  await expect(page.getByRole('group', { name: 'Selected invoices' })).toContainText('1 invoice selected:')
  await tick.uncheck()
  await expect(page.getByRole('group', { name: 'Selected invoices' })).toHaveCount(0)
  // A click anywhere in a row opens the invoice.
  await page.getByRole('table', { name: 'Invoices' }).getByRole('row').nth(1).getByRole('cell').last().click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/\d/)
})

test('sales invoices are paid by hand and by a bank transaction, and the overview follows', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Payments Co')
  const uri = administrationUri(page)
  const api = `/api/v2${uri}`
  const contact = await (await page.request.post(`${api}/contacts`, { data: { contact: { company_name: 'Example Shop B.V.', send_invoices_to_email: unique() } } })).json()
  const rates = await (await page.request.get(`${api}/tax_rates`)).json()
  const vat21 = rates.find((r: { tax_rate_type: string; percentage: string }) => r.tax_rate_type === 'sales_invoice' && r.percentage.startsWith('21')).id
  const draft = async (price: string) => {
    const created = await page.request.post(`${api}/sales_invoices`, { data: { sales_invoice: {
      contact_id: contact.id, prices_are_incl_tax: true, details_attributes: [{ description: 'Consulting', amount: '1', price, tax_rate_id: vat21 }],
    } } })
    expect(created.status()).toBe(201)
    return (await created.json()).id as string
  }

  // The first invoice is sent and paid by hand.
  const first = await draft('121')
  await page.goto(`${uri}/sales_invoices/${first}`)
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  const sending = page.getByRole('region', { name: 'Send invoice' })
  await sending.getByLabel('Send by').selectOption('Manual')
  await sending.getByRole('button', { name: 'Send invoice' }).click()
  await expect(page.locator('.badge', { hasText: 'Open' })).toBeVisible()
  const payments = page.getByRole('region', { name: 'Payments' })
  const form = payments.getByRole('form', { name: 'Register payment' })
  await expect(form.getByLabel('Amount')).toHaveValue('121')
  await form.getByRole('button', { name: 'Register payment' }).click()
  await expect(page.locator('.badge', { hasText: 'Paid' })).toBeVisible()
  await expect(payments.getByRole('table', { name: 'Payments' }).getByRole('row', { name: /Bank transfer/ })).toContainText('€121.00')

  // The second waits for its money, which the overview counts.
  const second = await draft('1000')
  const sent = await page.request.patch(`${api}/sales_invoices/${second}/send_invoice`, { data: { sales_invoice_sending: { delivery_method: 'Manual' } } })
  expect(sent.status()).toBe(200)
  const number = (await sent.json()).invoice_id as string
  await page.goto(`${uri}/feed`)
  const sales = page.getByRole('region', { name: 'Sales invoices and external invoices' })
  await expect(sales.getByRole('link', { name: /To receive\s*— 1\s*€1,000\.00/ })).toBeVisible()

  // A bank transaction of the same amount pays it.
  const account = await (await page.request.post(`${api}/financial_accounts`, { data: { financial_account: { name: 'Main bank', identifier: 'NL91ABNA0417164300' } } })).json()
  const today = new Date().toISOString().slice(0, 10)
  const camt = readFileSync(join(__dirname, '../../internal/bankfile/testdata/camt053.xml'), 'utf8').replace(/2026-1[01]-\d\d/g, today)
  const imported = await page.request.post(`${api}/financial_accounts/${account.id}/import`, { data: Buffer.from(camt), headers: { 'Content-Type': 'application/octet-stream' } })
  expect(imported.status()).toBe(201)
  const incoming = (await imported.json()).financial_statements[0].financial_mutations.find((m: { amount: string }) => m.amount === '1000.0')
  await page.goto(`${uri}/financial_mutations/${incoming.id}`)
  const link = page.getByRole('form', { name: 'Link to an invoice' })
  await link.getByLabel('Find an invoice').fill(number)
  await link.getByRole('radio', { name: new RegExp(number) }).check()
  await link.getByRole('button', { name: 'Link invoice' }).click()
  const paid = page.getByRole('table', { name: 'Invoices paid' })
  await expect(paid.getByRole('link', { name: number })).toBeVisible()
  await expect(page.getByRole('form', { name: 'Link to an invoice' })).toBeHidden()
  await paid.getByRole('link', { name: number }).click()
  await expect(page.locator('.badge', { hasText: 'Paid' })).toBeVisible()
  await expect(page.getByRole('table', { name: 'Payments' }).getByRole('link', { name: 'Bank transaction' })).toBeVisible()

  await page.goto(`${uri}/feed`)
  await expect(sales.getByRole('link', { name: /To receive\s*— 0\s*€0\.00/ })).toBeVisible()
})

test('an external invoice is uploaded, filled in, booked and paid by a bank transaction', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Webshop Co')
  const uri = administrationUri(page)
  const api = `/api/v2${uri}`
  const created = await page.request.post(`${api}/contacts`, { data: { contact: { company_name: 'Example Shop B.V.' } } })
  expect(created.status()).toBe(201)

  const menu = page.getByRole('navigation', { name: 'Administration menu' })
  await menu.getByRole('button', { name: 'Invoices' }).click()
  await menu.getByRole('link', { name: 'External invoices' }).click()
  await expect(page.getByText(/No external invoices yet/)).toBeVisible()

  // The webshop's PDF becomes a new invoice to fill in.
  await page.getByLabel('Upload invoices').setInputFiles({ name: 'order-1001.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4\n% order 1001\n') })
  await expect(page.locator('.badge', { hasText: 'New' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'View order-1001.pdf' })).toBeVisible()
  const form = page.getByRole('form', { name: 'External invoice' })
  await form.getByRole('combobox', { name: 'Contact' }).click()
  await page.getByRole('searchbox', { name: 'Find a contact' }).fill('Example')
  await page.getByRole('option', { name: 'Example Shop B.V.' }).click()
  await form.getByLabel('Reference').fill('WEB-1001')
  await form.getByLabel('Prices include VAT').check()
  const row = form.getByRole('table', { name: 'Lines' }).getByRole('row').nth(1)
  await row.getByLabel('Description').fill('Sofa')
  await row.getByLabel('Price').fill('1000')
  const vat21 = (await row.getByLabel('VAT rate').locator('option', { hasText: '21%' }).first().getAttribute('value')) ?? ''
  await row.getByLabel('VAT rate').selectOption(vat21)
  await form.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'WEB-1001' })).toBeVisible()
  await expect(page.locator('.badge', { hasText: 'Open' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Payments' })).toBeVisible()

  // A bank transaction of the same amount pays it.
  const account = await (await page.request.post(`${api}/financial_accounts`, { data: { financial_account: { name: 'Main bank', identifier: 'NL91ABNA0417164300' } } })).json()
  const today = new Date().toISOString().slice(0, 10)
  const camt = readFileSync(join(__dirname, '../../internal/bankfile/testdata/camt053.xml'), 'utf8').replace(/2026-1[01]-\d\d/g, today)
  const imported = await page.request.post(`${api}/financial_accounts/${account.id}/import`, { data: Buffer.from(camt), headers: { 'Content-Type': 'application/octet-stream' } })
  expect(imported.status()).toBe(201)
  const incoming = (await imported.json()).financial_statements[0].financial_mutations.find((m: { amount: string }) => m.amount === '1000.0')
  await page.goto(`${uri}/financial_mutations/${incoming.id}`)
  const link = page.getByRole('form', { name: 'Link to an invoice' })
  await link.getByLabel('Find an invoice').fill('WEB-1001')
  await link.getByRole('radio', { name: /WEB-1001/ }).check()
  await link.getByRole('button', { name: 'Link invoice' }).click()
  const paid = page.getByRole('table', { name: 'Invoices paid' })
  await paid.getByRole('link', { name: 'WEB-1001' }).click()
  await expect(page.locator('.badge', { hasText: 'Paid' })).toBeVisible()
  await expect(page.getByRole('table', { name: 'Payments' }).getByRole('link', { name: 'Bank transaction' })).toBeVisible()

  await page.goto(`${uri}/external_sales_invoices`)
  await expect(page.getByRole('table', { name: 'External invoices' }).getByRole('link', { name: 'WEB-1001' })).toBeVisible()
})

test('a draft invoice is made recurring, scheduled and deactivated', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Subscription Co')
  const uri = administrationUri(page)
  const api = `/api/v2${uri}`
  const contact = await (await page.request.post(`${api}/contacts`, { data: { contact: { company_name: 'Example Shop B.V.', send_invoices_to_email: unique() } } })).json()
  const rates = await (await page.request.get(`${api}/tax_rates`)).json()
  const vat21 = rates.find((r: { tax_rate_type: string; percentage: string }) => r.tax_rate_type === 'sales_invoice' && r.percentage.startsWith('21')).id
  const created = await page.request.post(`${api}/sales_invoices`, { data: { sales_invoice: {
    contact_id: contact.id, reference: 'Contract 7', details_attributes: [{ description: 'Hosting', amount: '1', price: '100', tax_rate_id: vat21 }],
  } } })
  expect(created.status()).toBe(201)
  const draft = (await created.json()).id as string

  const menu = page.getByRole('navigation', { name: 'Administration menu' })
  await menu.getByRole('button', { name: 'Invoices' }).click()
  await menu.getByRole('link', { name: 'Recurring invoices' }).click()
  await expect(page.getByText(/No recurring invoices yet/)).toBeVisible()

  // The draft is the template: its contact, reference and lines are copied.
  await page.goto(`${uri}/sales_invoices/${draft}`)
  await page.getByRole('button', { name: 'More actions' }).click()
  await page.getByRole('link', { name: 'Make recurring' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'New recurring invoice' })).toBeVisible()
  const form = page.getByRole('form', { name: 'Recurring invoice' })
  await expect(form.getByLabel('Reference')).toHaveValue('Contract 7')
  const next = new Date()
  next.setMonth(next.getMonth() + 1, 1)
  const first = `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, '0')}-01`
  await form.getByLabel('First invoice on').fill(first)
  await form.getByLabel('Every', { exact: true }).fill('3')
  await form.getByLabel('Description').fill('Hosting {period}')
  await form.getByLabel('Send each invoice automatically').check()
  await expect(form.getByRole('table', { name: 'Totals' }).getByRole('row', { name: /^Total/ })).toContainText('€121.00')
  await form.getByRole('button', { name: 'Save recurring invoice' }).click()

  await expect(page.getByRole('heading', { level: 1, name: 'Example Shop B.V.' })).toBeVisible()
  await expect(page).toHaveURL(/\/recurring_sales_invoices\/\d+$/)
  const details = page.getByRole('region', { name: 'Recurring invoice details' })
  await expect(details).toContainText('Every 3 months')
  await expect(details).toContainText('Yes')
  await expect(page.getByText('No invoices made yet.')).toBeVisible()

  await page.getByRole('link', { name: 'To Recurring invoices' }).click()
  const table = page.getByRole('table', { name: 'Recurring invoices' })
  await expect(table.getByRole('row')).toHaveCount(2)
  await expect(table.getByRole('row').nth(1)).toContainText('Every 3 months')
  await expect(table.getByRole('row').nth(1)).toContainText('€121.00')

  await table.getByRole('link', { name: 'Example Shop B.V.' }).click()
  await page.getByRole('button', { name: 'More actions' }).click()
  await page.getByRole('button', { name: 'Deactivate' }).click()
  await expect(page.getByText('This recurring invoice is deactivated and makes no more invoices.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save recurring invoice' })).toBeHidden()

  await page.getByRole('link', { name: 'To Recurring invoices' }).click()
  await expect(page.getByText(/No recurring invoices yet/)).toBeVisible()
  await page.getByRole('button', { name: 'Active recurring invoices' }).click()
  const filters = page.getByRole('search', { name: 'Recurring invoice filters' })
  await filters.getByRole('radio', { name: 'Inactive' }).check()
  await filters.getByRole('button', { name: 'Filter' }).click()
  await expect(page.getByRole('table', { name: 'Recurring invoices' }).getByRole('row')).toHaveCount(2)
})

test('a project and a product are added, listed and the product repriced', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Example Shop B.V.')
  const menu = page.getByRole('navigation', { name: 'Administration menu' })

  await menu.getByRole('link', { name: 'Projects' }).click()
  await expect(page.getByText(/No projects yet/)).toBeVisible()
  await page.getByRole('link', { name: 'Add project' }).click()
  const project = page.getByRole('form', { name: 'Project' })
  await project.getByLabel('Name').fill('Website redesign')
  await project.getByLabel('Budget').fill('40')
  await project.getByRole('button', { name: 'Save project' }).click()
  await expect(page).toHaveURL(/\/\d+\/projects\/\d+$/)
  await expect(page.getByRole('heading', { level: 1, name: 'Website redesign' })).toBeVisible()
  await page.getByRole('link', { name: 'To Projects' }).click()
  await expect(page.getByRole('table', { name: 'Projects' }).getByRole('row').nth(1)).toContainText('40 hours')

  await menu.getByRole('button', { name: 'Products' }).click()
  await menu.getByRole('link', { name: 'Overview' }).last().click()
  await expect(page.getByText(/No products yet/)).toBeVisible()
  await page.getByRole('link', { name: 'Add product' }).click()
  const product = page.getByRole('form', { name: 'Product' })
  await product.getByLabel('Title').fill('Website hosting')
  await product.getByLabel('Price').fill('50')
  await product.getByLabel('Category').selectOption({ index: 1 })
  await product.getByRole('button', { name: 'Save product' }).click()
  await expect(page).toHaveURL(/\/\d+\/products\/\d+$/)
  await expect(page.getByRole('heading', { level: 1, name: 'Website hosting' })).toBeVisible()

  await page.getByRole('link', { name: 'To Products' }).click()
  const table = page.getByRole('table', { name: 'Products' })
  await expect(table.getByRole('row').nth(1)).toContainText('€50.00')
  await table.getByRole('link', { name: 'Website hosting' }).click()
  await page.getByRole('form', { name: 'Product' }).getByLabel('Price').fill('60')
  await page.getByRole('form', { name: 'Product' }).getByRole('button', { name: 'Save product' }).click()
  await expect(page.getByRole('region', { name: 'Product details' })).toContainText('€60.00')
})

test('a quote is written, sent, accepted and billed as an invoice', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Quote Co')
  const uri = administrationUri(page)
  const created = await page.request.post(`/api/v2${uri}/contacts`, { data: { contact: { company_name: 'Example Shop B.V.', send_estimates_to_email: unique() } } })
  expect(created.status()).toBe(201)

  const menu = page.getByRole('navigation', { name: 'Administration menu' })
  await menu.getByRole('link', { name: 'Quotes' }).click()
  await expect(page.getByText(/No quotes yet/)).toBeVisible()
  await page.getByRole('link', { name: 'New quote' }).click()
  const form = page.getByRole('form', { name: 'Quote' })
  await form.getByRole('combobox', { name: 'Contact' }).click()
  await page.getByRole('searchbox', { name: 'Find a contact' }).fill('Example')
  await page.getByRole('option', { name: 'Example Shop B.V.' }).click()
  await form.getByLabel('Reference').fill('Project X')
  await form.getByLabel('Text above the lines').fill('Here is our offer.')
  const row = form.getByRole('table', { name: 'Lines' }).getByRole('row').nth(1)
  await row.getByLabel('Description').fill('Website redesign')
  await row.getByLabel('Price').fill('300')
  await row.getByLabel('VAT rate').selectOption((await row.getByLabel('VAT rate').locator('option', { hasText: '21%' }).first().getAttribute('value')) ?? '')
  await expect(form.getByRole('table', { name: 'Totals' }).getByRole('row', { name: /^Total/ })).toContainText('€363.00')
  await form.getByRole('button', { name: 'Save quote' }).click()
  await expect(page).toHaveURL(/\/\d+\/estimates\/\d+$/)
  await expect(page.getByRole('heading', { level: 1, name: /^Draft quote/ })).toBeVisible()

  await page.getByRole('button', { name: 'Mark as sent' }).click()
  await expect(page.locator('.badge', { hasText: 'Open' })).toBeVisible()
  await expect(page.getByRole('heading', { level: 1 })).not.toHaveText(/Draft/)
  const pdf = await page.request.get((await page.getByRole('button', { name: 'More actions' }).click(), await page.getByRole('link', { name: 'Download PDF' }).getAttribute('href')) ?? '')
  expect(pdf.status()).toBe(200)
  expect(pdf.headers()['content-type']).toContain('application/pdf')
  await page.getByRole('button', { name: 'Mark as accepted' }).click()
  await expect(page.locator('.badge', { hasText: 'Accepted' })).toBeVisible()

  await page.getByRole('button', { name: 'Bill', exact: true }).click()
  await expect(page).toHaveURL(/\/\d+\/sales_invoices\/\d+$/)
  await expect(page.getByRole('heading', { level: 1, name: /^Draft/ })).toBeVisible()
  await expect(page.getByRole('table', { name: 'Totals' }).getByRole('row', { name: /^Total/ })).toContainText('€363.00')

  await menu.getByRole('link', { name: 'Quotes' }).click()
  const table = page.getByRole('table', { name: 'Quotes' })
  await expect(table.getByRole('row')).toHaveCount(2)
  await expect(table.getByRole('row').nth(1)).toContainText('Billed')
})

test('time is recorded with a timer, and a task list is ticked off', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Example Shop B.V.')
  const menu = page.getByRole('navigation', { name: 'Administration menu' })

  await menu.getByRole('button', { name: 'Timesheets' }).click()
  await menu.getByRole('link', { name: 'Register' }).click()
  await expect(page.getByText(/No time recorded this year yet/)).toBeVisible()
  await page.getByRole('link', { name: 'Add time' }).click()
  const form = page.getByRole('form', { name: 'Time entry' })
  await form.getByLabel('Description').fill('Support call')
  await form.getByRole('button', { name: 'Save time' }).click()
  await expect(page).toHaveURL(/\/\d+\/time_entries\/\d+$/)
  const details = page.getByRole('region', { name: 'Time details' })
  await expect(details).toContainText('Running')
  await page.getByRole('button', { name: 'More actions' }).click()
  await page.getByRole('button', { name: 'Stop timer' }).click()
  await expect(details).toContainText('Open')
  await page.getByRole('link', { name: 'To Timesheets' }).click()
  await expect(page.getByRole('table', { name: 'Timesheets' }).getByRole('row').nth(1)).toContainText('Support call')

  await menu.getByRole('button', { name: 'Task lists' }).click()
  await menu.getByRole('link', { name: 'Task lists' }).last().click()
  await expect(page.getByText(/No task lists yet/)).toBeVisible()
  await page.getByRole('link', { name: 'Add task list' }).click()
  await page.getByRole('form', { name: 'Task list' }).getByLabel('Name').fill('Month end')
  await page.getByRole('form', { name: 'Task list' }).getByRole('button', { name: 'Save task list' }).click()
  await expect(page).toHaveURL(/\/\d+\/task_lists\/\d+$/)
  const add = page.getByRole('form', { name: 'Add a task to Tasks' })
  await add.getByLabel('New task').fill('Reconcile the bank')
  await add.getByRole('button', { name: 'Add task' }).click()
  await page.getByRole('checkbox', { name: 'Reconcile the bank' }).click()
  await expect(page.getByRole('checkbox', { name: 'Reconcile the bank' })).toBeChecked()
  await expect(page.getByRole('region', { name: 'Task list details' })).toContainText('Completed')
})

test('a purchase invoice is booked through the API, paid by a bank transaction and an upload waits to be processed', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Buyer Co')
  const uri = administrationUri(page)
  const api = `/api/v2${uri}`
  const supplier = await (await page.request.post(`${api}/contacts`, { data: { contact: { company_name: 'Supplier B.V.' } } })).json()
  const rates = await (await page.request.get(`${api}/tax_rates`)).json()
  const vat21 = rates.find((r: { tax_rate_type: string; percentage: string }) => r.tax_rate_type === 'purchase_invoice' && r.percentage.startsWith('21')).id
  const today = new Date().toISOString().slice(0, 10)
  const created = await page.request.post(`${api}/documents/purchase_invoices.json`, { data: { purchase_invoice: {
    contact_id: supplier.id, reference: 'INV-2026-77', date: today, prices_are_incl_tax: true,
    details_attributes: [{ description: 'Paper', price: '25.50', tax_rate_id: vat21 }],
  } } })
  expect(created.status()).toBe(201)

  const menu = page.getByRole('navigation', { name: 'Administration menu' })
  await menu.getByRole('button', { name: 'Incoming' }).click()
  await menu.getByRole('link', { name: 'Purchase invoices' }).click()
  const table = page.getByRole('table', { name: 'Purchase invoices' })
  await expect(table.getByRole('link', { name: 'INV-2026-77' })).toBeVisible()
  await expect(table.locator('.badge', { hasText: 'Open' })).toBeVisible()

  // The outgoing bank transaction pays it, booked against the creditors.
  const account = await (await page.request.post(`${api}/financial_accounts`, { data: { financial_account: { name: 'Main bank', identifier: 'NL91ABNA0417164300' } } })).json()
  const camt = readFileSync(join(__dirname, '../../internal/bankfile/testdata/camt053.xml'), 'utf8').replace(/2026-1[01]-\d\d/g, today)
  const imported = await page.request.post(`${api}/financial_accounts/${account.id}/import`, { data: Buffer.from(camt), headers: { 'Content-Type': 'application/octet-stream' } })
  expect(imported.status()).toBe(201)
  const outgoing = (await imported.json()).financial_statements[0].financial_mutations.find((m: { amount: string }) => m.amount === '-25.5')
  const doc = await created.json()
  const linked = await page.request.patch(`${api}/financial_mutations/${outgoing.id}/link_booking`, { data: { booking_type: 'Document', booking_id: doc.id } })
  expect(linked.status()).toBe(200)
  expect((await linked.json()).state).toBe('processed')
  await table.getByRole('link', { name: 'INV-2026-77' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'INV-2026-77' })).toBeVisible()
  await expect(page.locator('.badge', { hasText: 'Paid' })).toBeVisible()

  // An upload waits on the overview until it is processed.
  await menu.getByRole('link', { name: 'Uploads' }).click()
  await page.getByLabel('Upload documents').setInputFiles({ name: 'bill-78.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4\n% bill 78\n') })
  await expect(page.getByRole('table', { name: 'Uploads to process' }).getByRole('link', { name: 'bill-78.pdf' })).toBeVisible()
  await page.goto(`${uri}/feed`)
  await expect(page.getByRole('region', { name: 'Incoming' }).getByRole('link', { name: /To process\s*1/ })).toBeVisible()
})
