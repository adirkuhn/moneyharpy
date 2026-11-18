import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'
import i18n from '../i18n'
import { eventText, snake } from './text'
import { nextCursor as fromLink } from './queries'

describe('event texts', () => {
  const t = i18n.getFixedT('en')
  it('names the record type and what happened, as Moneybird', () => {
    expect(snake('ExternalSalesInvoice')).toBe('external_sales_invoice')
    expect(eventText('SalesInvoice', 'sales_invoice_created', t)).toBe('Invoice created')
    expect(eventText('SalesInvoice', 'sales_invoice_send_manually', t)).toBe('Invoice manually marked as sent')
    expect(eventText('ExternalSalesInvoice', 'external_sales_invoice_state_changed_to_paid', t)).toBe('External invoice has been paid')
    expect(eventText('Payment', 'payment_registered', t)).toBe('Payment registered')
    expect(eventText('TimeEntry', 'time_entry_updated', t)).toBe('Time entry updated')
    expect(eventText('Contact', 'contact_destroyed', t)).toBe('Contact deleted')
    expect(eventText('Asset', 'company_assets_asset_created', t)).toBe('Asset created')
    expect(eventText('Widget', 'something_odd', t)).toBe('Something odd')
  })

  it('reads the cursor of the next page from the Link header', () => {
    expect(fromLink('<https://example.com/api/v2/11/events?cursor=abc&per_page=50>; rel="next"')).toBe('abc')
    expect(fromLink(null)).toBeUndefined()
  })
})

describe('events page', () => {
  it('lists what happened this month with who did it', async () => {
    const seen: URL[] = []
    server.use(
      signedIn(),
      listing([administration({ id: '11', name: 'Harpy B.V.' })]),
      http.get('*/api/v2/administrations/11', () => HttpResponse.json(administration({ id: '11', name: 'Harpy B.V.' }))),
      http.get('*/api/v2/11/users', () => HttpResponse.json([{ id: '7', name: 'Jan Voorbeeld' }])),
      http.get('*/api/v2/11/contacts/301', () => HttpResponse.json({ id: '301', company_name: 'Example Shop B.V.', firstname: null, lastname: null })),
      http.get('*/api/v2/11/events', ({ request }) => {
        const url = new URL(request.url)
        seen.push(url)
        if (url.searchParams.get('cursor') === 'older') {
          return HttpResponse.json([{ id: '3', entity_type: 'Contact', entity_id: '302', action: 'contact_destroyed', user_id: '7', created_at: '2026-11-01T08:00:00Z' }])
        }
        return HttpResponse.json([
          { id: '2', entity_type: 'Contact', entity_id: '301', action: 'contact_created', user_id: '7', created_at: '2026-11-18T10:12:00Z' },
          { id: '1', entity_type: 'Payment', entity_id: '9', link_entity_type: 'SalesInvoice', link_entity_id: null, action: 'payment_registered', user_id: '7', created_at: '2026-11-17T19:14:00Z' },
        ], { headers: { Link: '<http://localhost/api/v2/11/events?cursor=older&per_page=50>; rel="next"' } })
      }),
    )
    renderAt('/11/events')
    const table = await screen.findByRole('table', { name: 'Events' })
    expect(seen.at(-1)?.searchParams.get('period')).toBe('this_month')
    expect(screen.getByRole('button', { name: 'Events from this month' })).toBeInTheDocument()
    const created = within(table).getByRole('row', { name: /Contact created/ })
    expect(await within(created).findByRole('link', { name: 'Contact Example Shop B.V.' })).toHaveAttribute('href', '/11/contacts/301')
    expect(created).toHaveTextContent(/Jan Voorbeeld.*2026-11-18.*JV/)
    expect(within(table).getByRole('row', { name: /Payment registered/ })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Older events' }))
    expect(await within(table).findByRole('row', { name: /Contact deleted/ })).toBeInTheDocument()
    expect(seen.at(-1)?.searchParams.get('cursor')).toBe('older')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Events' })).toHaveAttribute('aria-current', 'page')
  })
})
