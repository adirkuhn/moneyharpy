import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LedgerAccount } from '../bookkeeping/queries'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import type { JournalDocument } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'

const account = (over: Partial<LedgerAccount>): LedgerAccount => ({
  id: '1201', administration_id: '11', name: 'Rent', account_type: 'expenses', account_id: '4110', parent_id: null, active: true,
  allowed_document_types: ['purchase_invoice', 'general_journal_document', 'financial_mutation'], financial_account_id: null, created_at: stamp,
  updated_at: stamp, taxonomy_item: null, uri: '/11/ledger_accounts/1201', description: null, system_role: null, template: null, ...over,
})
const rent = account({})
const bank = account({ id: '1101', name: 'Main bank', account_type: 'current_assets', account_id: '1010', uri: '/11/ledger_accounts/1101' })
const deposits = account({ id: '1301', name: 'Private deposits', account_type: 'equity', account_id: '0520', uri: '/11/ledger_accounts/1301' })
const debtors = account({ id: '1302', name: 'Accounts receivable', account_type: 'current_assets', account_id: '1300', system_role: 'accounts_receivable', uri: '/11/ledger_accounts/1302' })

const values = (pairs: [string, string][]) => ({ ledger_accounts: pairs.map(([ledger_account_id, value]) => ({ ledger_account_id, value })) })

const doc = (over: Partial<JournalDocument> = {}): JournalDocument => ({
  id: '801', administration_id: '11', reference: 'Rent paid privately', date: '2026-11-05', journal_type: null, created_at: stamp, updated_at: stamp,
  version: 1, notes: [], attachments: [], events: [], uri: '/11/documents/general_journal_documents/801',
  general_journal_document_entries: [
    { id: '811', administration_id: '11', ledger_account_id: '1201', project_id: null, contact_id: null, debit: '400.0', credit: '0.0', row_order: 1, description: null, created_at: stamp, updated_at: stamp },
    { id: '812', administration_id: '11', ledger_account_id: '1301', project_id: null, contact_id: null, debit: '0.0', credit: '400.0', row_order: 2, description: null, created_at: stamp, updated_at: stamp },
  ],
  ...over,
})

function loaded(seen: URL[] = []) {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([rent, bank, deposits, debtors])),
    http.get('*/api/v2/11/reports/general_ledger', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({
        debit_sums: values([['1101', '1000.0'], ['1201', '400.0']]),
        credit_sums: values([['1101', '25.5'], ['1301', '400.0']]),
        start_balance: values([['1101', '0.0'], ['1201', '0.0'], ['1301', '0.0']]),
        final_balance: values([['1101', '974.5'], ['1201', '400.0'], ['1301', '-400.0']]),
      })
    }),
    http.get('*/api/v2/11/reports/journal_entries', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json([
        { id: '901', date: '2026-11-03', document_type: 'FinancialMutation', document_id: '701', ledger_account_id: '1101', project_id: null, contact_id: null, tax_rate_id: null, financial_account_id: '501', amount: '1000.0', description: 'Invoice 1' },
      ], { headers: { 'X-Total-Count': '1' } })
    }),
    http.get('*/api/v2/11/documents/general_journal_documents', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json([doc()], { headers: { 'X-Total-Count': '1' } })
    }),
    http.get('*/api/v2/11/documents/general_journal_documents/801', () => HttpResponse.json(doc())),
  ]
}

describe('trial balance', () => {
  it('lists every category with its movements and balances', async () => {
    server.use(...loaded())
    renderAt('/11/report/general_ledger')
    const table = await screen.findByRole('table', { name: 'Trial balance' })
    const row = within(table).getByRole('row', { name: /Main bank/ })
    expect(within(row).getByText('€1,000.00')).toBeInTheDocument()
    expect(within(row).getByText('€25.50')).toBeInTheDocument()
    expect(within(row).getByText('€974.50')).toBeInTheDocument()
    expect(within(table).getByRole('row', { name: /Total/ })).toHaveTextContent('€1,400.00')
    expect(within(table).queryByRole('row', { name: /Accounts receivable/ })).toBeNull()
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Trial balance' })).toHaveAttribute('aria-current', 'page')
  })

  it('changes the period', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/report/general_ledger')
    await screen.findByRole('table', { name: 'Trial balance' })
    expect(seen.at(-1)?.searchParams.get('period')).toBe('this_month')
    await userEvent.click(screen.getByRole('button', { name: 'This month' }))
    const panel = screen.getByRole('search', { name: 'Report filters' })
    await userEvent.click(within(panel).getByRole('link', { name: 'Last year' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('period')).toBe('prev_year'))
    expect(screen.getByRole('button', { name: 'Last year' })).toBeInTheDocument()
  })

  it('reports over a range of months picked in the period filter', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/report/general_ledger?period=202601..202601')
    await screen.findByRole('table', { name: 'Trial balance' })
    expect(seen.at(-1)?.searchParams.get('period')).toBe('202601..202601')
    await userEvent.click(screen.getByRole('button', { name: 'Jan 2026' }))
    const picker = screen.getByRole('group', { name: 'Period' })
    // A report always covers a period.
    expect(within(picker).queryByRole('button', { name: 'All periods' })).toBeNull()
    await userEvent.click(within(picker).getByRole('button', { name: 'February 2026' }))
    await userEvent.click(within(picker).getByRole('button', { name: 'April 2026' }))
    await userEvent.click(screen.getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('period')).toBe('202602..202604'))
    expect(screen.getByRole('button', { name: 'Feb 2026 – Apr 2026' })).toBeInTheDocument()
  })

  it('opens the bookings of a category', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/report/general_ledger')
    await userEvent.click(within(await screen.findByRole('table', { name: 'Trial balance' })).getByRole('link', { name: /Main bank/ }))
    const entries = await screen.findByRole('table', { name: 'Bookings' })
    expect(within(entries).getByText('Invoice 1')).toBeInTheDocument()
    expect(within(entries).getByRole('link', { name: 'Bank transaction' })).toHaveAttribute('href', '/11/financial_mutations/701')
    expect(seen.at(-1)?.searchParams.get('ledger_account_id')).toBe('1101')
  })
})

describe('journal entries', () => {
  it('lists the journal entries made by hand', async () => {
    server.use(...loaded())
    renderAt('/11/documents/general_journal_documents')
    const table = await screen.findByRole('table', { name: 'Journal entries' })
    expect(within(table).getByRole('link', { name: 'Rent paid privately' })).toHaveAttribute('href', '/11/documents/general_journal_documents/801')
    expect(within(table).getByText('€400.00')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'New journal entry' })).toHaveAttribute('href', '/11/documents/general_journal_documents/new')
  })

  it('lists the journal entries of every period', async () => {
    const seen: URL[] = []
    server.use(...loaded(seen))
    renderAt('/11/documents/general_journal_documents')
    await screen.findByRole('table', { name: 'Journal entries' })
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('period:this_year')
    await userEvent.click(screen.getByRole('button', { name: 'This year' }))
    await userEvent.click(within(screen.getByRole('group', { name: 'Period' })).getByRole('button', { name: 'All periods' }))
    await userEvent.click(screen.getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('filter')).toBe('state:all'))
    expect(screen.getByRole('button', { name: 'All periods' })).toBeInTheDocument()
  })

  it('books a balanced journal entry', async () => {
    let sent: { general_journal_document?: { general_journal_document_entries_attributes?: unknown[] } & Record<string, unknown> } = {}
    server.use(...loaded(), http.post('*/api/v2/11/documents/general_journal_documents', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(doc(), { status: 201 })
    }))
    renderAt('/11/documents/general_journal_documents/new')
    await userEvent.type(await screen.findByLabelText('Reference'), 'Rent paid privately')
    await userEvent.clear(screen.getByLabelText('Date'))
    await userEvent.type(screen.getByLabelText('Date'), '2026-11-05')
    const rows = screen.getByRole('table', { name: 'Rows' })
    const first = within(rows).getAllByRole('row')[1]!
    const second = within(rows).getAllByRole('row')[2]!
    await userEvent.selectOptions(within(first).getByLabelText('Category'), '1201')
    await userEvent.type(within(first).getByLabelText('Debit'), '400')
    await userEvent.selectOptions(within(second).getByLabelText('Category'), '1301')
    await userEvent.type(within(second).getByLabelText('Credit'), '300')
    expect(screen.getByText(/Difference/)).toHaveTextContent('€100.00')
    expect(screen.getByRole('button', { name: 'Save journal entry' })).toBeDisabled()
    await userEvent.clear(within(second).getByLabelText('Credit'))
    await userEvent.type(within(second).getByLabelText('Credit'), '400')
    await userEvent.click(screen.getByRole('button', { name: 'Save journal entry' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/documents/general_journal_documents/801'))
    expect(sent.general_journal_document).toMatchObject({ reference: 'Rent paid privately', date: '2026-11-05' })
    expect(sent.general_journal_document?.general_journal_document_entries_attributes).toEqual([
      { ledger_account_id: '1201', debit: '400', credit: '', description: '' },
      { ledger_account_id: '1301', debit: '', credit: '400', description: '' },
    ])
  })

  it('offers no debtors or creditors, which invoices settle', async () => {
    server.use(...loaded())
    renderAt('/11/documents/general_journal_documents/new')
    const rows = await screen.findByRole('table', { name: 'Rows' })
    const select = within(within(rows).getAllByRole('row')[1]!).getByLabelText('Category')
    expect(await within(select).findByRole('option', { name: /Rent/ })).toBeInTheDocument()
    expect(within(select).queryByRole('option', { name: /Accounts receivable/ })).toBeNull()
  })

  it('shows a journal entry and deletes it', async () => {
    server.use(...loaded(), http.delete('*/api/v2/11/documents/general_journal_documents/801', () => new HttpResponse(null, { status: 204 })))
    renderAt('/11/documents/general_journal_documents/801')
    expect(await screen.findByRole('heading', { level: 1, name: 'Rent paid privately' })).toBeInTheDocument()
    const rows = screen.getByRole('table', { name: 'Rows' })
    expect(await within(rows).findByRole('option', { name: /Rent/, selected: true })).toBeInTheDocument()
    await chooseMore('Delete')
    await userEvent.click(within(screen.getByRole('alertdialog', { name: 'Delete this journal entry? This cannot be undone.' })).getByRole('button', { name: 'Delete permanently' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/documents\/general_journal_documents$/))
  })
})
