import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { server } from '../test/server'
import { ContactHoverCard, HoverCard } from './ContactHoverCard'

const twoYearsAgo = new Date(Date.now() - 2.5 * 365 * 24 * 3600 * 1000).toISOString()

const shop = {
  id: '701', administration_id: '11', company_name: 'Example Shop B.V.', firstname: null, lastname: null, country: 'NL',
  sepa_iban: 'NL91ABNA0417164300', email: 'info@example.com', phone: '', address1: '', zipcode: '', city: '', created_at: twoYearsAgo,
}

/** The contact and how many sales invoices and incoming documents it has, counting what is asked. */
function contact(asked: string[] = []) {
  return [
    http.get('*/api/v2/11/contacts/701', () => {
      asked.push('contact')
      return HttpResponse.json(shop)
    }),
    http.get('*/api/v2/11/sales_invoices', ({ request }) => {
      const url = new URL(request.url)
      asked.push(`invoices ${url.searchParams.get('filter')} ${url.searchParams.get('per_page')}`)
      return HttpResponse.json([], { headers: { 'X-Total-Count': '4' } })
    }),
    http.get('*/api/v2/11/documents', ({ request }) => {
      const url = new URL(request.url)
      asked.push(`documents ${url.searchParams.get('filter')} ${url.searchParams.get('per_page')}`)
      return HttpResponse.json([], { headers: { 'X-Total-Count': '7' } })
    }),
  ]
}

function show(node: React.ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('the hover card of a contact', () => {
  it('shows the contact, its IBAN and email and how many invoices and documents it has, once hovered', async () => {
    const asked: string[] = []
    server.use(...contact(asked))
    show(<ContactHoverCard administrationId="11" contact={{ id: '701', company_name: 'Example Shop B.V.', firstname: null, lastname: null }} />)
    const link = screen.getByRole('link', { name: 'Example Shop B.V.' })
    expect(link).toHaveAttribute('href', '/11/contacts/701')
    // Nothing is asked until the card shows.
    expect(asked).toEqual([])
    await userEvent.hover(link)
    const card = await screen.findByRole('dialog', { name: 'Example Shop B.V.' })
    expect(within(card).getByRole('link', { name: 'Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/701')
    expect(await within(card).findByText('Created over 2 years ago')).toBeInTheDocument()
    expect(within(card).getByText('IBAN').nextElementSibling).toHaveTextContent('NL91ABNA0417164300')
    expect(within(card).getByText('Email').nextElementSibling).toHaveTextContent('info@example.com')
    expect(within(card).queryByText('Phone')).toBeNull()
    expect(within(card).queryByText('Address')).toBeNull()
    await waitFor(() => expect(within(card).getByText('Invoices').previousElementSibling).toHaveTextContent('4'))
    await waitFor(() => expect(within(card).getByText('Documents').previousElementSibling).toHaveTextContent('7'))
    expect(asked).toContain('invoices state:all,contact_id:701 1')
    expect(asked).toContain('documents state:all,type:all,contact_id:701 1')
    // The card stays while the pointer is on it, and goes when it leaves.
    await userEvent.hover(card)
    expect(screen.getByRole('dialog', { name: 'Example Shop B.V.' })).toBeInTheDocument()
    await userEvent.unhover(card)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Example Shop B.V.' })).toBeNull())
  })

  it('shows on focus and hides with Escape', async () => {
    server.use(...contact())
    show(<ContactHoverCard administrationId="11" contact={{ id: '701', company_name: 'Example Shop B.V.', firstname: null, lastname: null }} />)
    await userEvent.tab()
    expect(screen.getByRole('link', { name: 'Example Shop B.V.' })).toHaveFocus()
    expect(await screen.findByRole('dialog', { name: 'Example Shop B.V.' })).toBeInTheDocument()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('dialog', { name: 'Example Shop B.V.' })).toBeNull()
  })

  it('waits a moment before it shows, so passing over a name shows nothing', async () => {
    server.use(...contact())
    show(<HoverCard administrationId="11" contactId="701" name="Example Shop B.V.">{() => <span>Example Shop B.V.</span>}</HoverCard>)
    const name = screen.getByText('Example Shop B.V.')
    await userEvent.hover(name)
    await userEvent.unhover(name)
    await new Promise((r) => setTimeout(r, 300))
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
