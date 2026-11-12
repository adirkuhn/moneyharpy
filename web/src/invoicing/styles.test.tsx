import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { defaultStyle, loaded, otherStyle } from './testkit'

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

  it('shows what is not available yet, as Moneybird lays it out', async () => {
    server.use(...loaded())
    renderAt('/11/document_styles/702/edit/document')
    await userEvent.click(await screen.findByRole('button', { name: 'Info under customer address on invoices' }))
    expect(screen.getAllByText(/not available in MoneyHarpy yet/).length).toBeGreaterThan(0)
    await userEvent.click(screen.getByRole('button', { name: 'Advanced' }))
    expect(screen.getByRole('switch', { name: 'Show QR-code' })).toBeDisabled()

    await userEvent.click(screen.getByRole('link', { name: 'Email' }))
    expect(await screen.findByText(/Styling emails is not available in MoneyHarpy yet/)).toBeInTheDocument()
    for (const name of ['Header', 'Footer', 'General settings']) {
      expect(screen.getByRole('button', { name })).toBeDisabled()
    }
    await userEvent.click(screen.getByRole('link', { name: 'Public views' }))
    expect(await screen.findByRole('button', { name: 'Sender address invoices' })).toBeDisabled()
    await userEvent.click(screen.getByRole('link', { name: 'Online sales' }))
    expect(await screen.findByRole('button', { name: 'Basic settings' })).toBeDisabled()
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
