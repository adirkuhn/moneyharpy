import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { invoiceSettings, server } from '../test/server'
import { loaded, otherStyle } from './testkit'

describe('invoices and estimates settings', () => {
  it('shows every part, with placeholders for what is not available', async () => {
    server.use(...loaded())
    renderAt('/11/settings/invoices_and_estimates')
    expect(await screen.findByRole('heading', { name: 'Invoices and estimates', level: 1 })).toBeInTheDocument()
    for (const name of ['Document styles', 'Workflows', 'Numbering', 'Peppol & UBL', 'Custom fields', 'Forward documents', 'Online payment methods', 'Payments without proof']) {
      expect(screen.getByRole('region', { name })).toBeInTheDocument()
    }
    const peppol = screen.getByRole('region', { name: 'Peppol & UBL' })
    expect(within(peppol).getByRole('button', { name: 'Activate Peppol' })).toBeDisabled()
    expect(within(peppol).getByText(/needs a Peppol access point/)).toBeInTheDocument()
    const payments = screen.getByRole('region', { name: 'Online payment methods' })
    expect(within(payments).getByRole('button', { name: 'Offer online payment methods' })).toBeDisabled()
    expect(within(payments).getByText(/need a payment provider/)).toBeInTheDocument()
    const methods = within(payments).getByRole('table', { name: 'Workflows' })
    expect(await within(methods).findByText('Standaard (NL)')).toBeInTheDocument()
    expect(within(methods).queryByText('Export')).toBeInTheDocument()
    expect(screen.getByText(/come with purchase invoices/)).toBeInTheDocument()
    expect(within(screen.getByRole('group', { name: 'Invoice number' })).getByLabelText('Next id')).toHaveValue('0001')
    const nav = screen.getByRole('navigation', { name: 'Settings' })
    expect(within(nav).getByRole('link', { name: 'Numbering' })).toHaveAttribute('href', '/11/settings/invoices_and_estimates#numbering')
  })

  it('lists document styles and switches the default', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ ...otherStyle, default: true })
    }))
    renderAt('/11/settings/invoices_and_estimates')
    const block = await screen.findByRole('region', { name: 'Document styles' })
    expect(await within(block).findByText('Selected as default layout')).toBeInTheDocument()
    expect(within(block).getByRole('link', { name: 'The Best' })).toHaveAttribute('href', '/11/document_styles/702/edit')
    expect(within(block).getByRole('link', { name: 'Add' })).toHaveAttribute('href', '/11/document_styles/new')
    await userEvent.click(within(block).getByRole('button', { name: 'Select as default layout' }))
    await waitFor(() => expect(sent).toEqual({ document_style: { default: true } }))
  })

  it('lists workflows, and the default cannot be deleted', async () => {
    let deleted = false
    server.use(...loaded(), http.delete('*/api/v2/11/workflows/603', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/settings/invoices_and_estimates')
    const table = await screen.findByRole('table', { name: 'Invoice workflows' })
    expect(await within(table).findByRole('link', { name: 'Standaard (NL)' })).toHaveAttribute('href', '/11/workflows/601')
    expect(within(table).getByRole('button', { name: 'Default workflow cannot be deleted' })).toBeDisabled()
    await userEvent.click(within(table).getByRole('button', { name: 'Delete Export' }))
    expect(deleted).toBe(false)
    await userEvent.click(within(table).getByRole('button', { name: 'Confirm delete Export' }))
    await waitFor(() => expect(deleted).toBe(true))
    expect(screen.getByRole('table', { name: 'Quote workflows' })).toBeInTheDocument()
  })

  it('builds numbers from a template with a live example', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/invoice_settings', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(invoiceSettings({ invoice_id_template: 'F{id}', invoice_id_example: 'F0001' }))
    }))
    renderAt('/11/settings/invoices_and_estimates')
    const card = await screen.findByRole('group', { name: 'Invoice number' })
    const template = within(card).getByLabelText('Template')
    await userEvent.clear(template)
    await userEvent.type(template, 'F{{id}')
    expect(within(card).getByText('Example: F0001')).toBeInTheDocument()
    await userEvent.click(within(card).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ invoice_settings: { invoice_id_template: 'F{id}', invoice_sequence_id: '0001' } }))
  })

  it('inserts tags into the template', async () => {
    server.use(...loaded())
    renderAt('/11/settings/invoices_and_estimates')
    const card = await screen.findByRole('group', { name: 'Quote number' })
    await userEvent.click(within(card).getByRole('button', { name: 'Insert tag' }))
    await userEvent.click(within(card).getByRole('button', { name: 'Quarter' }))
    expect(within(card).getByLabelText('Template')).toHaveValue('{year}-{id}{quarter}')
  })

  it('saves switches straight away', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/invoice_settings', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(invoiceSettings({ payments_without_proof: false }))
    }))
    renderAt('/11/settings/invoices_and_estimates')
    const block = await screen.findByRole('region', { name: 'Payments without proof' })
    await userEvent.click(within(block).getByRole('switch', { name: 'Allow' }))
    await waitFor(() => expect(sent).toEqual({ invoice_settings: { payments_without_proof: false } }))
  })

  it('forwards copies and only offers reminders with an invoice address', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/invoice_settings', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(invoiceSettings())
    }))
    renderAt('/11/settings/invoices_and_estimates')
    const block = await screen.findByRole('region', { name: 'Forward documents' })
    const reminders = within(block).getByRole('checkbox', { name: 'Also send reminders to this address.' })
    expect(reminders).toBeDisabled()
    await userEvent.type(within(block).getByLabelText('Invoices'), 'books@example.com')
    await userEvent.click(reminders)
    await userEvent.click(within(block).getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(sent).toEqual({ invoice_settings: { invoice_bcc_email_address: 'books@example.com', invoice_reminders_bcc_enabled: true, estimate_bcc_email_address: '', order_bcc_email_address: '' } }),
    )
  })

  it('orders, edits and deletes custom fields', async () => {
    let order: unknown
    let deleted = false
    server.use(
      ...loaded(),
      http.put('*/api/v2/11/custom_fields/order', async ({ request }) => {
        order = await request.json()
        return new HttpResponse(null, { status: 204 })
      }),
      http.delete('*/api/v2/11/custom_fields/802', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/settings/invoices_and_estimates')
    const table = await screen.findByRole('table', { name: 'Sender address' })
    expect(within(table).getByRole('link', { name: 'Edit Slogan' })).toHaveAttribute('href', '/11/custom_fields/801/edit')
    expect(within(table).getByRole('button', { name: 'Move Slogan up' })).toBeDisabled()
    await userEvent.click(within(table).getByRole('button', { name: 'Move Slogan down' }))
    await waitFor(() => expect(order).toEqual({ custom_fields_order: { source: 'identity', ids: ['802', '801'] } }))
    await userEvent.click(within(table).getByRole('button', { name: 'Delete Motto' }))
    expect(deleted).toBe(false)
    await userEvent.click(within(table).getByRole('button', { name: 'Confirm delete Motto' }))
    await waitFor(() => expect(deleted).toBe(true))
  })
})
