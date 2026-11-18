import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18n from 'i18next'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { chooseMore } from '../test/menu'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import { periodName } from './period'
import type { VatPreview, VatReturn } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

const box = (b: string, base: string, tax: string, correction = ['0', '0']) => ({
  box: b, base, tax, period_base: String(Number(base) - Number(correction[0])), period_tax: String(Number(tax) - Number(correction[1])),
  correction_base: correction[0] ?? '0', correction_tax: correction[1] ?? '0',
})

const figures = {
  kind: 'vat_return' as const, period_from: '2026-07-01', period_to: '2026-09-30', granularity: 'quarter' as const, deadline: '2026-10-31',
  boxes: [box('1a', '1202', '252'), box('1b', '51498', '4633', ['197', '17']), box('5b', '0', '4440')],
  corrections: [{ period_from: '2026-04-01', period_to: '2026-06-30', boxes: [{ box: '1b', base: '197', tax: '17' }] }],
  icp_lines: [], owed: '4885', input_tax: '4440', total: '445',
}

const filed = (over: Partial<VatReturn> = {}): VatReturn => ({
  ...figures, id: '901', administration_id: '11', uri: '/11/declarations/901', sent_on: '2026-10-29', state: 'to_pay', paid: '0.0', unpaid: '445.0',
  payments: [], created_at: '2026-10-29T10:00:00Z', updated_at: '2026-10-29T10:00:00Z', ...over,
})

const preview: VatPreview = {
  ...figures, corrections: [], boxes: [box('1a', '1202', '252'), box('5b', '0', '42')], owed: '252', input_tax: '42', total: '210',
  checks: { open_transactions: 136, untaxed_purchases: '124.71', bookings_without_vat: 0, unprocessed_documents: 0 },
}

const q3 = { period_from: '2026-07-01', period_to: '2026-09-30', granularity: 'quarter' as const }
const q2 = { period_from: '2026-04-01', period_to: '2026-06-30', granularity: 'quarter' as const }

/** The periods each kind can be filed for. */
const periods = (byKind: Record<string, (typeof q3)[]>) =>
  http.get('*/api/v2/11/declarations/periods', ({ request }) => HttpResponse.json(byKind[new URL(request.url).searchParams.get('kind') ?? ''] ?? []))

function loaded() {
  return [
    signedIn(),
    listing([harpy]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    http.get('*/api/v2/11/declarations', () => HttpResponse.json([
      filed(), filed({ id: '902', period_from: '2026-04-01', period_to: '2026-06-30', state: 'paid', sent_on: '2026-07-28' }),
      filed({ id: '903', period_from: '2025-07-01', period_to: '2025-09-30', state: 'to_receive', sent_on: '2025-10-04' }),
    ])),
    http.get('*/api/v2/11/declarations/901', () => HttpResponse.json(filed())),
    http.get('*/api/v2/11/ledger_accounts', () => HttpResponse.json([{ id: '1510', name: 'Payable VAT' }])),
    http.get('*/api/v2/11/reports/journal_entries', () => HttpResponse.json([
      { id: '1', date: '2026-09-30', document_type: 'VatDocument', document_id: '901', ledger_account_id: '1510', amount: '4885.0' },
    ])),
  ]
}

describe('VAT returns', () => {
  it('names periods as Moneybird does', () => {
    const t = i18n.getFixedT('en')
    expect(periodName('2026-07-01', '2026-09-30', t, 'en')).toBe('Quarter 3 2026')
    expect(periodName('20261101', '20261130', t, 'en')).toBe('November 2026')
    expect(periodName('2026-01-01', '2026-12-31', t, 'en')).toBe('2026')
  })

  it('lists the returns per year with their state', async () => {
    server.use(...loaded())
    renderAt('/11/declarations')
    const year = await screen.findByRole('table', { name: 'VAT returns of 2026' })
    const rows = within(year).getAllByRole('row')
    expect(within(rows[1] as HTMLElement).getAllByRole('cell')[1]).toHaveTextContent(/^Quarter 3$/)
    expect(rows[1]).toHaveTextContent('2026-10-29')
    expect(rows[1]).toHaveTextContent('To pay')
    expect(within(rows[1] as HTMLElement).getByRole('link', { name: 'VAT return' })).toHaveAttribute('href', '/11/declarations/901')
    expect(rows[2]).toHaveTextContent('Paid')
    expect(screen.getByRole('table', { name: 'VAT returns of 2025' })).toHaveTextContent('To receive')
    expect(screen.getByRole('link', { name: 'Declare new VAT return' })).toHaveAttribute('href', '/11/declarations/new')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'VAT returns' })).toHaveAttribute('aria-current', 'page')
  })

  it('declares a return by hand: type, period, checks and the amounts to copy', async () => {
    let sent: { vat_return?: Record<string, unknown> } = {}
    server.use(...loaded(),
      periods({ vat_return: [q3, q2], supplement: [q2], icp_declaration: [q3] }),
      http.get('*/api/v2/11/declarations/preview', ({ request }) => {
        expect(new URL(request.url).searchParams.get('period')).toBe('20260701..20260930')
        return HttpResponse.json(preview)
      }),
      http.post('*/api/v2/11/declarations', async ({ request }) => {
        sent = await request.json() as typeof sent
        return HttpResponse.json(filed(), { status: 201 })
      }),
    )
    renderAt('/11/declarations/new')
    await userEvent.click(await screen.findByRole('radio', { name: /Supplementary VAT return/ }))
    await userEvent.click(screen.getByRole('radio', { name: /^VAT return/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByRole('combobox')).toHaveDisplayValue('Quarter 3 2026')
    expect(screen.getByText(/Your current VAT period is Quarter/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'VAT return' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))

    const checks = await screen.findByRole('list', { name: 'Check' })
    expect(checks).toHaveTextContent('136 bank transactions are still open')
    expect(checks).toHaveTextContent('€124.71 found. This amount is not included in the declaration.')
    expect(checks).toHaveTextContent('2 checks passed')
    const boxes = screen.getByRole('table', { name: 'VAT return Quarter 3 2026' })
    expect(within(boxes).getByRole('row', { name: /1a\. Supplies \/ services taxed at high rate/ })).toHaveTextContent('€1,202€252')
    expect(within(boxes).getByRole('row', { name: /5a\./ })).toHaveTextContent('€252')
    expect(within(boxes).getByRole('row', { name: /Total/ })).toHaveTextContent('€210')
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))

    const submit = await screen.findByRole('form', { name: 'Submit' })
    expect(within(submit).getByRole('link', { name: 'Mijn Belastingdienst Zakelijk' })).toHaveAttribute('href', expect.stringContaining('belastingdienst.nl'))
    const copy = within(submit).getByRole('table', { name: 'Amounts to fill in' })
    expect(within(copy).getByRole('row', { name: /1e\./ })).toHaveTextContent('€0-')
    expect(within(copy).getByRole('row', { name: /5b\./ })).toHaveTextContent('-€42')
    expect(submit).toHaveTextContent('Submit the declaration as soon as the total matches €210.')
    expect(within(submit).getByRole('button', { name: 'Mark as filed' })).toBeDisabled()
    await userEvent.click(within(submit).getByRole('checkbox', { name: /I filed the VAT return of Harpy B.V. for Quarter 3 2026/ }))
    await userEvent.click(within(submit).getByRole('button', { name: 'Mark as filed' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/declarations/901'))
    expect(sent.vat_return).toEqual({ kind: 'vat_return', period: '20260701..20260930' })
  })

  it('cannot choose a supplement without corrections nor an ICP declaration without EU supplies', async () => {
    server.use(...loaded(), periods({ vat_return: [q3], supplement: [], icp_declaration: [] }))
    renderAt('/11/declarations/new')
    expect(await screen.findByRole('radio', { name: /Supplementary VAT return/ })).toBeDisabled()
    expect(screen.getByRole('radio', { name: /ICP declaration/ })).toBeDisabled()
    expect(screen.getByRole('radio', { name: /^VAT return/ })).toBeChecked()
    expect(screen.getByText('The "Supplementary VAT return" option cannot be selected because there are no corrections.')).toBeInTheDocument()
    expect(screen.getByText(/You cannot submit an ICP declaration because no goods or services have been supplied abroad/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next' })).toBeEnabled()
  })

  it('files an ICP declaration with goods or services per customer', async () => {
    let sent: { vat_return?: Record<string, unknown> } = {}
    const icp: VatPreview = {
      ...preview, kind: 'icp_declaration', boxes: [], owed: '0', input_tax: '0', total: '0',
      icp_lines: [{ contact_id: '5', name: 'Beispiel GmbH', country: 'DE', vat_number: 'DE123456789', kind: 'services', amount: '500' }],
    }
    server.use(...loaded(),
      periods({ vat_return: [q3], supplement: [], icp_declaration: [q3] }),
      http.get('*/api/v2/11/declarations/preview', () => HttpResponse.json(icp)),
      http.post('*/api/v2/11/declarations', async ({ request }) => {
        sent = await request.json() as typeof sent
        return HttpResponse.json(filed({ kind: 'icp_declaration', state: 'submitted' }), { status: 201 })
      }),
    )
    renderAt('/11/declarations/new')
    await userEvent.click(await screen.findByRole('radio', { name: /ICP declaration/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByRole('combobox')
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByRole('table', { name: 'ICP declaration' })).toHaveTextContent('DE123456789')
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    const submit = await screen.findByRole('form', { name: 'Submit' })
    await userEvent.selectOptions(within(submit).getByRole('combobox', { name: 'What Beispiel GmbH was supplied' }), 'goods')
    await userEvent.click(within(submit).getByRole('checkbox'))
    await userEvent.click(within(submit).getByRole('button', { name: 'Mark as filed' }))
    await waitFor(() => expect(sent.vat_return).toEqual({ kind: 'icp_declaration', period: '20260701..20260930', icp_lines: [{ contact_id: '5', kind: 'goods' }] }))
  })

  it('shows a return: what to pay by when, the corrected boxes and the bookings', async () => {
    server.use(...loaded())
    renderAt('/11/declarations/901')
    expect(await screen.findByRole('heading', { name: 'VAT return - Quarter 3 2026' })).toBeInTheDocument()
    const status = screen.getByRole('region', { name: 'Status' })
    expect(status).toHaveTextContent('€445.00')
    expect(status).toHaveTextContent(/days overdue/)
    expect(status).toHaveTextContent('To pay')
    expect(status).toHaveTextContent('2026-10-31')
    expect(within(status).getByRole('link', { name: 'Link bank transaction' })).toHaveAttribute('href', '/11/financial_mutations')
    const boxes = screen.getByRole('table', { name: 'VAT return Quarter 3 2026' })
    const rows = within(boxes).getAllByRole('row')
    const at = rows.findIndex((r) => r.textContent?.startsWith('1b.'))
    expect(rows[at]).toHaveTextContent('€51,498€4,633')
    expect(rows[at + 1]).toHaveTextContent('Quarter 3 2026€51,301€4,616')
    expect(rows[at + 2]).toHaveTextContent('Corrections€197€17')
    expect(within(rows[rows.findIndex((r) => r.textContent?.startsWith('1c.'))] as HTMLElement).queryByText('€0')).toBeNull()
    expect(screen.getByRole('complementary', { name: 'History' })).toHaveTextContent('Filed at the tax authority')
    await userEvent.click(screen.getByRole('button', { name: 'Journal entries' }))
    expect(await screen.findByRole('table', { name: 'Journal entries' })).toHaveTextContent('Payable VAT€4,885.00')
  })

  it('deletes a return recorded by mistake', async () => {
    let deleted = false
    server.use(...loaded(), http.delete('*/api/v2/11/declarations/901', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/declarations/901')
    await screen.findByRole('heading', { name: 'VAT return - Quarter 3 2026' })
    await chooseMore('Delete')
    await userEvent.click(screen.getByRole('button', { name: 'Delete return' }))
    await waitFor(() => expect(deleted).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/declarations$/))
  })
})
