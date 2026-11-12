import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { exportFlow, loaded } from './testkit'

type Sent = { workflow?: Record<string, unknown> }

/** Serves the Export workflow and keeps what was saved. */
function patching(sent: Sent) {
  return http.patch('*/api/v2/11/workflows/603', async ({ request }) => {
    Object.assign(sent, (await request.json()) as Sent)
    return HttpResponse.json(exportFlow)
  })
}

describe('workflow form', () => {
  it('adds an invoice workflow with reminders, starting from the default payment conditions', async () => {
    let sent: Sent = {}
    server.use(...loaded(), http.post('*/api/v2/11/workflows', async ({ request }) => {
      sent = (await request.json()) as Sent
      return HttpResponse.json(exportFlow, { status: 201 })
    }))
    renderAt('/11/workflows/new/invoice_workflow')
    expect(await screen.findByRole('heading', { name: 'New invoice workflow', level: 1 })).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Name'), 'Export')
    const term = screen.getByLabelText('Payment term')
    await userEvent.clear(term)
    await userEvent.type(term, '30')
    expect((screen.getByRole('textbox', { name: 'Payment conditions on invoice' }) as HTMLTextAreaElement).value).toMatch(/^We kindly request you to pay/)
    await userEvent.click(screen.getByRole('button', { name: 'Add reminder' }))
    const reminder = screen.getByRole('group', { name: 'Reminder 1' })
    await userEvent.type(within(reminder).getByLabelText('Reminder text'), 'Please pay')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/invoices_and_estimates'))
    expect(sent.workflow).toMatchObject({
      type: 'InvoiceWorkflow', name: 'Export',
      steps_attributes: [{ due_interval: 30 }, { reminder_delay: 7, reminder_text: 'Please pay', reminder_auto_send: false }],
    })
    // Untouched, the server gives the workflow Moneybird's text in its language.
    expect(sent.workflow).not.toHaveProperty('payment_conditions')
  })

  it('has no invoice email or payment conditions for quotes', async () => {
    server.use(...loaded())
    renderAt('/11/workflows/new/estimate_workflow')
    expect(await screen.findByRole('heading', { name: 'New quote workflow', level: 1 })).toBeInTheDocument()
    expect(screen.queryByLabelText('Email subject')).toBeNull()
    expect(screen.queryByLabelText('Email text')).toBeNull()
    expect(screen.queryByLabelText('Payment conditions on invoice')).toBeNull()
  })
})

describe('workflow page', () => {
  it("shows the default workflow as Moneybird's workflow page", async () => {
    server.use(...loaded())
    renderAt('/11/workflows/601')
    expect(await screen.findByRole('heading', { name: 'Invoice workflow: Standaard (NL)', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Default workflow cannot be deleted' })).toBeDisabled()

    const general = screen.getByRole('region', { name: 'General settings' })
    expect(general).toHaveTextContent('NLEURPrices are excl. tax')
    expect(within(general).getAllByText('Not available in MoneyHarpy yet')).toHaveLength(3)
    const payment = screen.getByRole('region', { name: 'Payment settings' })
    expect(payment).toHaveTextContent('No direct debit method has been configured.')
    expect(payment).toHaveTextContent('QR-code')

    const send = screen.getByRole('region', { name: /^Send invoice/ })
    expect(send).toHaveTextContent('Day 1')
    expect(send).toHaveTextContent('Reminder expires after14 days')
    expect(within(send).getByText(/We verzoeken u vriendelijk het bovenstaande bedrag van € 1\.210,00 voor \d{2}-\d{2}-\d{4} .* omschrijving RF89CONCEPT\./)).toBeInTheDocument()
    // Reminders fold shut until Show more, as on Moneybird.
    const first = screen.getByRole('region', { name: /^1st reminder/ })
    expect(first).toHaveTextContent('Day 22')
    expect(within(first).queryByText('Manual')).toBeNull()
    await userEvent.click(within(first).getByRole('button', { name: 'Show more' }))
    expect(within(first).getByText('Automatic')).toBeInTheDocument()
    expect(within(first).getByText('Please pay')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: 'Payment' }))
    const panel = screen.getByRole('tabpanel', { name: 'Payment' })
    expect(within(panel).getByRole('region', { name: 'Payment succeeded' })).toHaveTextContent('No email is sent after payment.')
    expect(within(panel).getByRole('region', { name: 'Payment failed' })).toBeInTheDocument()
    expect(within(panel).getByRole('region', { name: 'Changed IBAN service' })).toBeInTheDocument()
  })

  it('goes back to the invoice it was opened from', async () => {
    server.use(...loaded())
    renderAt('/11/workflows/601?return_to=%2F11%2Fsales_invoices%2F902')
    await screen.findByRole('heading', { name: 'Invoice workflow: Standaard (NL)', level: 1 })
    expect(screen.getByRole('link', { name: 'To invoice' })).toHaveAttribute('href', '/11/sales_invoices/902')
  })

  it('edits the payment conditions and email of the Send invoice step with the textile editor', async () => {
    const sent: Sent = {}
    server.use(...loaded(), patching(sent))
    server.use(http.get('*/api/v2/11/workflows/603', () => HttpResponse.json({ ...exportFlow, payment_conditions: 'Pay soon', invoice_email_subject: 'Invoice {invoice_id}' })))
    renderAt('/11/workflows/603')
    await userEvent.click(await screen.findByRole('button', { name: 'Edit Send invoice' }))
    const form = screen.getByRole('form', { name: 'Send invoice' })
    const conditions = within(form).getByRole('textbox', { name: 'Payment conditions on invoice' }) as HTMLTextAreaElement
    conditions.setSelectionRange(4, 8)
    await userEvent.click(within(form).getByRole('button', { name: 'Bold' }))
    expect(conditions).toHaveValue('Pay *soon*')
    expect(within(form).getByLabelText('Email subject')).toHaveValue('Invoice {invoice_id}')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.workflow).toMatchObject({
      payment_conditions: 'Pay *soon*', invoice_email_subject: 'Invoice {invoice_id}',
      steps_attributes: [{ due_interval: 14 }, { due_interval: 0, reminder_delay: 7, reminder_text: 'Please pay', reminder_auto_send: true }],
    }))
    await waitFor(() => expect(screen.queryByRole('form', { name: 'Send invoice' })).toBeNull())
  })

  it('edits, adds and removes reminders', async () => {
    const sent: Sent = {}
    server.use(...loaded(), patching(sent))
    renderAt('/11/workflows/603')
    await userEvent.click(await screen.findByRole('button', { name: 'Edit 1st reminder' }))
    let form = screen.getByRole('form', { name: '1st reminder' })
    await userEvent.click(within(form).getByRole('radio', { name: 'Manual' }))
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.workflow?.steps_attributes).toEqual([{ due_interval: 14 }, { due_interval: 0, reminder_delay: 7, reminder_text: 'Please pay', reminder_auto_send: false }]))

    await userEvent.click(await screen.findByRole('button', { name: 'Add reminder' }))
    form = screen.getByRole('form', { name: '2nd reminder' })
    await userEvent.type(within(form).getByLabelText('Email message for this reminder'), 'Second notice')
    await userEvent.click(within(form).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.workflow?.steps_attributes).toHaveLength(3))
    expect((sent.workflow?.steps_attributes as unknown[])[2]).toMatchObject({ reminder_delay: 7, reminder_text: 'Second notice', reminder_auto_send: false })

    await userEvent.click(await screen.findByRole('button', { name: 'Edit 1st reminder' }))
    await userEvent.click(within(screen.getByRole('form', { name: '1st reminder' })).getByRole('button', { name: 'Remove reminder 1' }))
    await waitFor(() => expect(sent.workflow?.steps_attributes).toEqual([{ due_interval: 14 }]))
  })

  it('edits the general and payment settings, and deletes a workflow that is not the default', async () => {
    const sent: Sent = {}
    let deleted = false
    server.use(...loaded(), patching(sent), http.delete('*/api/v2/11/workflows/603', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/workflows/603')
    await userEvent.click(await screen.findByRole('button', { name: 'Edit general settings' }))
    const general = screen.getByRole('form', { name: 'General settings' })
    await userEvent.click(within(general).getByLabelText('Prices include VAT'))
    await userEvent.click(within(general).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.workflow).toMatchObject({ name: 'Export', prices_are_incl_tax: true, active: true, default: false }))

    await userEvent.click(await screen.findByRole('button', { name: 'Edit payment settings' }))
    const payment = screen.getByRole('form', { name: 'Payment settings' })
    await userEvent.click(within(payment).getByLabelText('Show a payment QR code on invoices'))
    await userEvent.click(within(payment).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.workflow).toMatchObject({ show_qr_code: false }))

    await userEvent.click(screen.getByRole('button', { name: 'Delete workflow' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete for good' }))
    await waitFor(() => expect(deleted).toBe(true))
  })
})
