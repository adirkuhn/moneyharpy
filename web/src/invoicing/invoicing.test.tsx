import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, documentStyle, identity, invoiceSettings, listing, server, signedIn, workflow } from '../test/server'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const invoiceFlow = workflow()
const quoteFlow = workflow({ id: '602', type: 'EstimateWorkflow', steps: [] })
const exportFlow = workflow({ id: '603', name: 'Export', default: false })
const defaultStyle = documentStyle()
const otherStyle = documentStyle({ id: '702', name: 'The Best', default: false })
const slogan = { id: '801', administration_id: '11', name: 'Slogan', source: 'identity' as const, uri: '/11/custom_fields/801' }
const motto = { id: '802', administration_id: '11', name: 'Motto', source: 'identity' as const, uri: '/11/custom_fields/802' }

const loaded = () => [
  signedIn(),
  listing([harpy]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
  http.get('*/api/v2/11/invoice_settings', () => HttpResponse.json(invoiceSettings())),
  http.get('*/api/v2/11/workflows', () => HttpResponse.json([quoteFlow, exportFlow, invoiceFlow])),
  http.get('*/api/v2/11/workflows/601', () => HttpResponse.json(invoiceFlow)),
  http.get('*/api/v2/11/workflows/603', () => HttpResponse.json(exportFlow)),
  http.get('*/api/v2/11/document_styles', () => HttpResponse.json([defaultStyle, otherStyle])),
  http.get('*/api/v2/11/document_styles/702', () => HttpResponse.json(otherStyle)),
  http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([slogan, motto])),
  http.get('*/api/v2/11/identities', () => HttpResponse.json([identity(), identity({ id: '501', company_name: 'Trade Name', default: false })])),
  http.get('*/api/onboarding/options', () =>
    HttpResponse.json({ countries: ['NL'], currencies: ['EUR', 'USD'], popular_currencies: ['EUR'], company_search_countries: [], address_lookup_countries: [] }),
  ),
]

describe('invoices and estimates settings', () => {
  it('shows every part, with placeholders for what is not available', async () => {
    server.use(...loaded())
    renderAt('/11/settings/invoices_and_estimates')
    expect(await screen.findByRole('heading', { name: 'Invoices and estimates', level: 1 })).toBeInTheDocument()
    for (const name of ['Document styles', 'Workflows', 'Numbering', 'Peppol & UBL', 'Custom fields', 'Forward documents', 'Online payment methods', 'Payments without proof']) {
      expect(screen.getByRole('region', { name })).toBeInTheDocument()
    }
    expect(screen.getByText(/needs a Peppol access point/)).toBeInTheDocument()
    expect(screen.getByText(/need a payment provider/)).toBeInTheDocument()
    expect(screen.getByText(/come with purchase invoices/)).toBeInTheDocument()
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
    await userEvent.selectOptions(within(card).getByLabelText('Insert tag'), 'quarter')
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

describe('document style form', () => {
  it('edits a style and its layout', async () => {
    let sent: { document_style?: Record<string, unknown> } = {}
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(otherStyle)
    }))
    renderAt('/11/document_styles/702/edit')
    expect(await screen.findByRole('heading', { name: 'Edit document style', level: 1 })).toBeInTheDocument()
    await userEvent.selectOptions(screen.getByLabelText('Sender address'), '501')
    await userEvent.selectOptions(screen.getByLabelText('Paper size'), 'letter')
    const block = screen.getByRole('group', { name: 'Invoices: Sender block' })
    await userEvent.click(within(block).getByRole('button', { name: 'Add line' }))
    await userEvent.selectOptions(within(block).getAllByLabelText('Field')[2] as HTMLElement, 'tax_number')
    await userEvent.click(within(block).getAllByRole('checkbox', { name: 'Show label' })[2] as HTMLElement)
    await userEvent.click(within(block).getByRole('button', { name: 'Remove line 2' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/invoices_and_estimates'))
    expect(sent.document_style).toMatchObject({
      identity_id: '501', paper_size: 'letter',
      invoice_sender_address: [{ field: 'company_name', bold: true, label: false }, { field: 'tax_number', bold: false, label: true }],
    })
  })

  it('uploads a logo as the request body', async () => {
    let contentType = ''
    let size = 0
    server.use(...loaded(), http.put('*/api/v2/11/document_styles/702/logo', async ({ request }) => {
      contentType = request.headers.get('Content-Type') ?? ''
      size = (await request.arrayBuffer()).byteLength
      return HttpResponse.json({ ...otherStyle, logo_hash: 'a'.repeat(64) })
    }))
    renderAt('/11/document_styles/702/edit')
    const input = await screen.findByLabelText('Upload Logo')
    await userEvent.upload(input, new File([new Uint8Array(10)], 'logo.png', { type: 'image/png' }))
    await waitFor(() => expect(contentType).toBe('image/png'))
    expect(size).toBe(10)
    expect(await screen.findByRole('img', { name: 'Current Logo' })).toHaveAttribute('src', expect.stringContaining('/document_styles/702/logo?v='))
  })

  it('asks to save a new style before uploading files', async () => {
    server.use(...loaded())
    renderAt('/11/document_styles/new')
    expect(await screen.findByText('Save the style first to add a logo or background.')).toBeInTheDocument()
    expect(screen.queryByLabelText('Upload Logo')).toBeNull()
  })
})
