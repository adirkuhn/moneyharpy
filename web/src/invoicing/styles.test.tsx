import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { defaultStyle, loaded, otherStyle, sentInvoice } from './testkit'

describe('document style editor', () => {
  const item = (list: HTMLElement, text: string) => within(list).getAllByRole('listitem').find((li) => li.textContent?.includes(text)) as HTMLElement
  const fieldNames = (list: HTMLElement) => within(list).getAllByRole('listitem').map((li) => within(li).getByRole('button', { name: /^Move / }).getAttribute('aria-label'))

  it('opens on the invoices and quotes tab, with the settings in sections and a preview', async () => {
    server.use(...loaded())
    renderAt('/11/document_styles/702/edit')
    expect(await screen.findByRole('heading', { name: 'The Best', level: 1 })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/document_styles/702/edit/document'))
    expect(screen.getByRole('link', { name: 'To Invoices and estimates' })).toHaveAttribute('href', '/11/settings/invoices_and_estimates#document-styles')
    const tabs = screen.getByRole('navigation', { name: 'Document style' })
    expect(within(tabs).getByRole('link', { name: 'Invoices and quotes' })).toHaveAttribute('aria-current', 'page')
    expect(within(tabs).getByRole('link', { name: 'Email' })).toHaveAttribute('href', '/11/document_styles/702/edit/email')
    for (const name of ['Logo', 'Font', 'Background image', 'Layout', 'Sender address invoices', 'Sender address quotes', 'Invoice metadata', 'Quote metadata', 'CSS', 'Advanced']) {
      expect(screen.getByRole('button', { name })).toHaveAttribute('aria-expanded', 'false')
    }
    expect(screen.getByRole('link', { name: 'Change name and sender address' })).toHaveAttribute('href', '/11/document_styles/702/edit_general')
    const invoice = screen.getByRole('document', { name: 'Invoice preview' })
    expect(within(invoice).getByText('Harpy B.V.')).toBeInTheDocument()
    expect(within(invoice).getByText('Dam 1')).toBeInTheDocument()
    expect(within(invoice).getByText(/Invoice 2026-0001/)).toBeInTheDocument()
    expect(screen.getByRole('document', { name: 'Quote preview' })).toBeInTheDocument()
  })

  it('arranges the sender address by keyboard and saves the style', async () => {
    let sent: { document_style?: Record<string, unknown> } = {}
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(otherStyle)
    }))
    renderAt('/11/document_styles/702/edit/document')
    await userEvent.click(await screen.findByRole('button', { name: 'Sender address invoices' }))
    const show = screen.getByRole('list', { name: 'Show' })
    const hidden = screen.getByRole('list', { name: "Don't show" })
    expect(fieldNames(show)).toEqual(['Move Company name', 'Move Address'])
    expect(within(item(show, 'Dam 1')).getByText('Address:')).toBeInTheDocument()

    within(item(show, 'Dam 1')).getByRole('button', { name: 'Move Address' }).focus()
    await userEvent.keyboard('{ArrowRight}')
    expect(fieldNames(show)).toEqual(['Move Company name'])
    expect(within(screen.getByRole('document', { name: 'Invoice preview' })).queryByText('Dam 1')).toBeNull()

    within(item(hidden, 'books@example.com')).getByRole('button', { name: 'Move Email address' }).focus()
    await userEvent.keyboard('{ArrowLeft}')
    expect(fieldNames(show)).toEqual(['Move Company name', 'Move Email address'])
    expect(within(show).getByRole('button', { name: 'Move Email address' })).toHaveFocus()
    await userEvent.keyboard('{ArrowUp}')
    expect(fieldNames(show)).toEqual(['Move Email address', 'Move Company name'])
    await userEvent.click(screen.getByRole('button', { name: 'Show label of Email address' }))
    expect(screen.getByRole('button', { name: 'Show label of Email address' })).toHaveAttribute('aria-pressed', 'true')

    await userEvent.click(screen.getByRole('button', { name: 'Font' }))
    await userEvent.selectOptions(screen.getByLabelText('Font size'), '12')
    await userEvent.click(screen.getByRole('button', { name: 'Advanced' }))
    await userEvent.click(screen.getByRole('radio', { name: 'Letter' }))
    await userEvent.click(screen.getByRole('switch', { name: 'Own stationery' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/invoices_and_estimates'))
    expect(sent.document_style).toMatchObject({
      font_size: 12, paper_size: 'letter', print_on_stationery: true,
      invoice_sender_address: [{ field: 'email', bold: false, label: true }, { field: 'company_name', bold: true, label: false }],
    })
    expect(sent.document_style).not.toHaveProperty('name')
  })

  it('places invoice details in the left and right columns', async () => {
    let sent: { document_style?: Record<string, unknown> } = {}
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(otherStyle)
    }))
    renderAt('/11/document_styles/702/edit/document')
    await userEvent.click(await screen.findByRole('button', { name: 'Invoice metadata' }))
    const left = screen.getByRole('list', { name: 'Left column' })
    expect(fieldNames(left)).toEqual(['Move Reference'])
    within(left).getByRole('button', { name: 'Move Reference' }).focus()
    await userEvent.keyboard('{ArrowRight}')
    expect(fieldNames(screen.getByRole('list', { name: 'Right column' }))).toEqual(['Move Invoice date', 'Move Due date', 'Move Reference'])
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(sent.document_style).toMatchObject({ invoice_metadata_left: [], invoice_metadata_right: [{ field: 'invoice_date' }, { field: 'due_date' }, { field: 'reference' }] }),
    )
  })

  it('uploads a logo as the request body', async () => {
    let contentType = ''
    let size = 0
    server.use(...loaded(), http.put('*/api/v2/11/document_styles/702/logo', async ({ request }) => {
      contentType = request.headers.get('Content-Type') ?? ''
      size = (await request.arrayBuffer()).byteLength
      return HttpResponse.json({ ...otherStyle, logo_hash: 'a'.repeat(64) })
    }))
    renderAt('/11/document_styles/702/edit/document')
    await userEvent.click(await screen.findByRole('button', { name: 'Logo' }))
    await userEvent.click(screen.getByRole('radio', { name: 'Centered' }))
    expect(screen.getByRole('radio', { name: 'Centered' })).toBeChecked()
    await userEvent.upload(screen.getByLabelText('Upload Logo'), new File([new Uint8Array(10)], 'logo.png', { type: 'image/png' }))
    await waitFor(() => expect(contentType).toBe('image/png'))
    expect(size).toBe(10)
    expect(await screen.findByRole('img', { name: 'Current Logo' })).toHaveAttribute('src', expect.stringContaining('/document_styles/702/logo?v='))
  })

  it('lays out the info under the customer address, italics, the sender width and the QR code', async () => {
    let sent: { document_style?: Record<string, unknown> } = {}
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(otherStyle)
    }))
    renderAt('/11/document_styles/702/edit/document')
    await userEvent.click(await screen.findByRole('button', { name: 'Info under customer address on invoices' }))
    const hidden = screen.getAllByRole('list', { name: "Don't show" })[0] as HTMLElement
    within(hidden).getByRole('button', { name: 'Move VAT number' }).focus()
    await userEvent.keyboard('{ArrowLeft}')
    await userEvent.click(screen.getByRole('button', { name: 'Show label of VAT number' }))
    const invoice = screen.getByRole('document', { name: 'Invoice preview' })
    expect(within(within(invoice).getByRole('list', { name: 'Info under customer address on invoices' })).getByText('VAT number:')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Sender address invoices' }))
    await userEvent.click(screen.getByRole('button', { name: 'Italic Company name' }))
    await userEvent.click(screen.getByRole('button', { name: 'Background image' }))
    await userEvent.click(screen.getByRole('checkbox', { name: 'Hide the sender address on invoices and quotes' }))
    expect(within(invoice).queryByRole('list', { name: 'Sender address' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Advanced' }))
    await userEvent.click(screen.getByRole('switch', { name: 'Show QR-code' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.document_style).toMatchObject({
      hide_sender_address: true, show_epc_code: false, sender_address_width: 200,
      invoice_recipient_address: [{ field: 'customer_tax_number', bold: false, italic: false, label: true }],
      invoice_sender_address: [{ field: 'company_name', bold: true, italic: true, label: false }, { field: 'address' }],
    }))
  })

  it('offers the custom fields of the sender in its address', async () => {
    server.use(...loaded())
    renderAt('/11/document_styles/702/edit/document')
    await userEvent.click(await screen.findByRole('button', { name: 'Sender address invoices' }))
    expect(within(screen.getByRole('list', { name: "Don't show" })).getByRole('button', { name: 'Move Slogan' })).toBeInTheDocument()
  })

  it('styles the email and previews it with the workflow text', async () => {
    let sent: { document_style?: Record<string, unknown> } = {}
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(otherStyle)
    }))
    renderAt('/11/document_styles/702/edit/email/en')
    const email = await screen.findByRole('article', { name: 'Email preview' })
    expect(within(email).getByText(/Please find attached invoice 2026-0001/)).toBeInTheDocument()
    expect(within(email).getByText('Harpy B.V. (books@example.com)')).toBeInTheDocument()
    expect(screen.getByText('Sent with MoneyHarpy')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Header' }))
    await userEvent.selectOptions(screen.getByLabelText('Header type'), 'background_color_with_text')
    await userEvent.type(screen.getByLabelText('Text'), 'Thanks for your order')
    expect(within(email).getByText('Thanks for your order')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Footer' }))
    await userEvent.selectOptions(screen.getByLabelText('Footer type'), 'signature')
    await userEvent.type(screen.getByLabelText('Title'), 'Ann de Vries')
    await userEvent.click(screen.getByRole('button', { name: 'General settings' }))
    await userEvent.selectOptions(screen.getByLabelText('Font'), 'georgia')
    await userEvent.click(screen.getByRole('switch', { name: 'MoneyHarpy label' }))
    expect(screen.queryByText('Sent with MoneyHarpy')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.document_style).toMatchObject({
      email_header_type: 'background_color_with_text', email_header_text: 'Thanks for your order', email_footer_type: 'signature',
      email_footer_profile_title: 'Ann de Vries', email_font_family: 'georgia', email_label_enabled: false,
    }))
  })

  it('refuses a colour that is not one and opens its part', async () => {
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', () =>
      HttpResponse.json({ error: { body_background_color: ['must be a colour such as #F7F9FC'] } }, { status: 422 })))
    renderAt('/11/document_styles/702/edit/email')
    await userEvent.click(await screen.findByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/Background colour must be a colour/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'General settings' })).toHaveAttribute('aria-expanded', 'true')
  })

  it('sets the public view and the online sales checkout', async () => {
    let sent: { document_style?: Record<string, unknown> } = {}
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(otherStyle)
    }))
    renderAt('/11/document_styles/702/edit/public_view/en')
    expect(await screen.findByRole('article', { name: 'Public view preview' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sender address invoices' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'General settings' }))
    await userEvent.selectOptions(screen.getByLabelText('Background colour'), '#D5E8FA')

    await userEvent.click(screen.getByRole('link', { name: 'Online sales' }))
    expect(await screen.findByRole('region', { name: 'Online sales preview' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Logo' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Basic settings' }))
    await userEvent.click(screen.getByRole('switch', { name: 'Phone number' }))
    expect(within(screen.getByRole('region', { name: 'Online sales preview' })).getByText('Phone number')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Favorite product settings' }))
    await userEvent.type(screen.getByLabelText('Label text'), 'Most chosen')
    expect(within(screen.getByRole('region', { name: 'Online sales preview' })).getByText('Most chosen')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.document_style).toMatchObject({
      public_view_background_color: '#D5E8FA', checkout_show_phone_field: true, checkout_favorite_label: 'Most chosen',
    }))
  })

  it('previews the latest sent invoice with its customer, lines and VAT', async () => {
    server.use(http.get('*/api/v2/11/sales_invoices', () => HttpResponse.json([sentInvoice])), ...loaded())
    renderAt('/11/document_styles/702/edit/document/en')
    const invoice = await screen.findByRole('document', { name: 'Invoice preview' })
    expect(within(invoice).getByText('Example Shop B.V.')).toBeInTheDocument()
    expect(within(invoice).getByText('Consulting')).toBeInTheDocument()
    expect(within(invoice).getByText(/Invoice 2026-0042/)).toBeInTheDocument()
    expect(within(invoice).getByText('€242.00')).toBeInTheDocument()
    expect(screen.getByText('The preview shows your latest sent invoice, 2026-0042.')).toBeInTheDocument()
  })

  it('cannot delete the default style', async () => {
    server.use(...loaded(), http.get('*/api/v2/11/document_styles/701', () => HttpResponse.json(defaultStyle)))
    renderAt('/11/document_styles/701/edit/document')
    expect(await screen.findByRole('button', { name: 'Default layout cannot be deleted' })).toBeDisabled()
  })

  it('deletes another style after confirming', async () => {
    let deleted = false
    server.use(...loaded(), http.delete('*/api/v2/11/document_styles/702', () => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/document_styles/702/edit/document')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete layout' }))
    await userEvent.click(screen.getByRole('button', { name: 'Confirm delete layout' }))
    await waitFor(() => expect(deleted).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/invoices_and_estimates'))
  })
})
