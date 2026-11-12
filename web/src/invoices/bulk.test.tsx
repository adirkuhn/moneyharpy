import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { draft, invoice, loaded } from './testkit'

describe('bulk actions in the list', () => {
  it('registers full payments, sends reminders and deletes drafts of the selected invoices', async () => {
    const paid: unknown[] = []
    let reminders: unknown
    let deleted = false
    const late = invoice({ id: '904', invoice_id: '2026-0004', state: 'late', total_unpaid: '100.0', uri: '/11/sales_invoices/904' })
    server.use(...loaded([invoice(), late, draft]))
    server.use(
      http.post('*/api/v2/11/sales_invoices/:id/payments', async ({ request, params }) => {
        paid.push({ id: params.id, ...((await request.json()) as object) })
        return HttpResponse.json({ id: '1' }, { status: 201 })
      }),
      http.post('*/api/v2/11/sales_invoices/send_reminders', async ({ request }) => {
        reminders = await request.json()
        return HttpResponse.json([late])
      }),
      http.delete('*/api/v2/11/sales_invoices/902', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/sales_invoices')
    const table = await screen.findByRole('table', { name: 'Invoices' })
    expect(screen.queryByRole('group', { name: 'Selected invoices' })).toBeNull()
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select 2026-0004' }))
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select Draft #3' }))
    let bar = screen.getByRole('group', { name: 'Selected invoices' })
    expect(within(bar).getByText('2 invoices selected:')).toBeInTheDocument()
    await userEvent.click(within(bar).getByRole('button', { name: 'Send reminders (1)' }))
    await waitFor(() => expect(reminders).toEqual({ sales_invoice_reminders: [{ contact_id: '701', workflow_id: '601', document_style_id: '701', identity_id: '461', sales_invoice_ids: ['904'] }] }))
    expect(await screen.findByText('Sent a reminder for 1 invoice.')).toBeInTheDocument()

    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select all invoices' }))
    bar = screen.getByRole('group', { name: 'Selected invoices' })
    await userEvent.click(within(bar).getByRole('button', { name: 'Register full payment (2)' }))
    await waitFor(() => expect(paid).toHaveLength(2))
    expect(paid).toEqual([
      { id: '901', payment: { payment_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/), price: '242.0' } },
      { id: '904', payment: { payment_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/), price: '100.0' } },
    ])
    expect(await screen.findByText('Registered the payments of 2 invoices.')).toBeInTheDocument()

    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select Draft #3' }))
    bar = screen.getByRole('group', { name: 'Selected invoices' })
    expect(within(bar).getByRole('button', { name: 'Register full payment (0)' })).toBeDisabled()
    await userEvent.click(within(bar).getByRole('button', { name: 'Delete drafts (1)' }))
    await userEvent.click(within(bar).getByRole('button', { name: 'Delete 1 draft permanently' }))
    await waitFor(() => expect(deleted).toBe(true))
    expect(await screen.findByText('Deleted 1 draft.')).toBeInTheDocument()
  })
})
