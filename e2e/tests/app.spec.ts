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
    },
  })
  expect(saved.status()).toBe(200)
  // Creates the bucket on first run.
  expect((await api.post('/api/instance/settings/test_storage')).status()).toBe(204)
  await api.dispose()
})

async function resetLinkFor(email: string): Promise<string> {
  const api = await playwrightRequest.newContext()
  let link = ''
  await expect
    .poll(async () => {
      const found = await (await api.get(`${mailpit}/api/v1/search?query=${encodeURIComponent(`to:${email}`)}`)).json()
      if (found.messages.length === 0) {
        return false
      }
      const message = await (await api.get(`${mailpit}/api/v1/message/${found.messages[0].ID}`)).json()
      link = /https?:\/\/\S+\/passwords\/edit\?token=\S+/.exec(message.Text)?.[0] ?? ''
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
  await expect(page.getByRole('heading', { name: 'Wizard Co' })).toBeVisible()

  const id = new URL(page.url()).pathname.slice(1)
  const administration = await (await page.request.get(`/api/v2/administrations/${id}`)).json()
  expect(administration).toMatchObject({ currency: 'USD', country: 'NL' })
  const company = await (await page.request.get(`/api/v2/${id}/identities/default.json`)).json()
  expect(company).toMatchObject({ company_name: 'Wizard Co', address1: 'Dam 1', zipcode: '1012JS', city: 'Amsterdam', country: 'NL' })
})

test('company details and sender addresses are managed in settings', async ({ page }) => {
  await register(page)
  await addAdministration(page, 'Settings Co')
  const uri = new URL(page.url()).pathname

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
  const uri = new URL(page.url()).pathname
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
  const uri = new URL(page.url()).pathname
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
  const uri = new URL(page.url()).pathname
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

  await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Bank' }).click()
  await page.getByRole('group', { name: 'State' }).getByText('All', { exact: true }).click()
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
})
