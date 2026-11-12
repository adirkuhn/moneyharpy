import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { exportFlow, loaded } from './testkit'

describe('workflow form', () => {
  it('adds an invoice workflow with reminders', async () => {
    let sent: { workflow?: Record<string, unknown> } = {}
    server.use(...loaded(), http.post('*/api/v2/11/workflows', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(exportFlow, { status: 201 })
    }))
    renderAt('/11/workflows/new/invoice_workflow')
    expect(await screen.findByRole('heading', { name: 'New invoice workflow', level: 1 })).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Name'), 'Export')
    const term = screen.getByLabelText('Payment term')
    await userEvent.clear(term)
    await userEvent.type(term, '30')
    await userEvent.click(screen.getByRole('button', { name: 'Add reminder' }))
    const reminder = screen.getByRole('group', { name: 'Reminder 1' })
    await userEvent.type(within(reminder).getByLabelText('Reminder text'), 'Please pay')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/invoices_and_estimates'))
    expect(sent.workflow).toMatchObject({
      type: 'InvoiceWorkflow', name: 'Export',
      steps_attributes: [{ due_interval: 30 }, { reminder_delay: 7, reminder_text: 'Please pay', reminder_auto_send: false }],
    })
  })

  it('keeps the default workflow active and undeletable', async () => {
    server.use(...loaded())
    renderAt('/11/workflows/601')
    expect(await screen.findByRole('heading', { name: 'Edit workflow', level: 1 })).toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toHaveValue('Standaard (NL)')
    expect(screen.getByRole('group', { name: 'Reminder 1' })).toBeInTheDocument()
    expect(screen.queryByRole('checkbox', { name: 'Active' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull()
    expect(screen.getByText(/This is the default workflow/)).toBeInTheDocument()
  })

  it('sets the email invoices are sent with', async () => {
    let sent: { workflow?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/workflows/603', () => HttpResponse.json({ ...exportFlow, invoice_email_subject: 'Invoice {invoice_id}' })),
      http.patch('*/api/v2/11/workflows/603', async ({ request }) => {
        sent = (await request.json()) as typeof sent
        return HttpResponse.json(exportFlow)
      }),
    )
    renderAt('/11/workflows/603')
    const subject = await screen.findByLabelText('Email subject')
    expect(subject).toHaveValue('Invoice {invoice_id}')
    expect(subject).toHaveAccessibleDescription(/Empty uses the default/)
    await userEvent.click(screen.getByLabelText('Email text'))
    await userEvent.paste('Dear {contact_name}, please pay {total}.')
    expect(screen.getByText(/\{payment_reference\}/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.workflow).toMatchObject({ invoice_email_subject: 'Invoice {invoice_id}', invoice_email_body: 'Dear {contact_name}, please pay {total}.' }))
  })

  it('has no invoice email for quotes', async () => {
    server.use(...loaded())
    renderAt('/11/workflows/new/estimate_workflow')
    expect(await screen.findByRole('heading', { name: 'New quote workflow', level: 1 })).toBeInTheDocument()
    expect(screen.queryByLabelText('Email subject')).toBeNull()
    expect(screen.queryByLabelText('Email text')).toBeNull()
  })
})
