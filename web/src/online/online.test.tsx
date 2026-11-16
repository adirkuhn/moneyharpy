import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { components } from '../api/schema'
import { renderAt } from '../test/render'
import { server } from '../test/server'

type OnlineEstimate = components['schemas']['OnlineEstimate']

const quote = (over: Partial<OnlineEstimate> = {}): OnlineEstimate => ({
  id: '971', estimate_id: '2026-0027', state: 'open', estimate_date: '2026-11-16', due_date: '2026-12-16', reference: 'Q-0001',
  currency: 'EUR', language: 'en', prices_are_incl_tax: false, pre_text: 'Dear Jan Voorbeeld,', post_text: 'Kind regards',
  details: [{ description: 'Advice', amount: '2', price: '150.0', total_price_excl_tax_with_discount: '300.0', tax_percentage: '21' }],
  total_price_excl_tax: '300.0', total_tax: '63.0', total_price_incl_tax: '363.0',
  tax_totals: [{ tax_percentage: '21', taxable_amount: '300.0', tax_amount: '63.0' }],
  sender: { company_name: 'Example Shop B.V.', address1: 'Example Street 1', zipcode: '1234 AB', city: 'Amsterdam', email: 'info@example.com', chamber_of_commerce: '12345678', tax_number: 'NL123456782B01' },
  contact_name: 'Jan Voorbeeld', signable: true, signed: false, signer_name: null, signed_at: null,
  ...over,
})

// jsdom draws nothing; the pad's picture is stubbed.
beforeEach(() => {
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
  vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/png;base64,SIGNED')
})
afterEach(() => vi.restoreAllMocks())

const draw = (pad: HTMLElement) => {
  fireEvent.pointerDown(pad, { clientX: 5, clientY: 5, pointerId: 1 })
  fireEvent.pointerMove(pad, { clientX: 40, clientY: 20, pointerId: 1 })
  fireEvent.pointerUp(pad, { clientX: 40, clientY: 20, pointerId: 1 })
}

describe('a quote online', () => {
  it('shows the quote to its contact, who accepts it with a signature', async () => {
    let signing: unknown
    server.use(
      http.get('*/api/online/estimates/971', ({ request }) =>
        new URL(request.url).searchParams.get('code') === '123456789012' ? HttpResponse.json(quote()) : HttpResponse.json({ error: 'Not found' }, { status: 404 })),
      http.post('*/api/online/estimates/971/sign', async ({ request }) => {
        signing = await request.json()
        return HttpResponse.json(quote({ state: 'accepted', signable: false, signed: true, signer_name: 'Jan Voorbeeld', signed_at: '2026-11-16T14:30:00Z' }))
      }),
    )
    renderAt('/online/estimates/971?code=123456789012')
    expect(await screen.findByRole('heading', { level: 1, name: 'Quote 2026-0027' })).toBeInTheDocument()
    const doc = screen.getByRole('document', { name: 'Quote 2026-0027' })
    expect(doc).toHaveTextContent('Example Shop B.V.')
    expect(doc).toHaveTextContent('Dear Jan Voorbeeld,')
    expect(within(doc).getByRole('table', { name: 'Quote lines' })).toHaveTextContent('Advice')
    expect(doc).toHaveTextContent('€363.00')
    expect(screen.getByRole('link', { name: 'Download PDF' })).toHaveAttribute('href', '/api/online/estimates/971/pdf?code=123456789012')

    const form = screen.getByRole('form', { name: 'Accept quote' })
    await userEvent.click(within(form).getByRole('button', { name: 'Accept and sign quote' }))
    expect(within(form).getByText('Sign in the field above.')).toBeInTheDocument()
    expect(signing).toBeUndefined()
    await userEvent.type(within(form).getByLabelText('Name'), 'Jan Voorbeeld')
    await userEvent.type(within(form).getByLabelText('Email address'), 'jan@example.com')
    draw(within(form).getByRole('img', { name: 'Signature' }))
    await userEvent.click(within(form).getByRole('button', { name: 'Accept and sign quote' }))
    await waitFor(() => expect(signing).toMatchObject({ name: 'Jan Voorbeeld', email: 'jan@example.com', signature_output: 'data:image/png;base64,SIGNED' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Accepted and signed by Jan Voorbeeld')
    expect(screen.queryByRole('form', { name: 'Accept quote' })).toBeNull()
  })

  it('says when the link no longer opens the quote', async () => {
    server.use(http.get('*/api/online/estimates/971', () => HttpResponse.json({ error: 'Not found' }, { status: 404 })))
    renderAt('/online/estimates/971?code=1')
    expect(await screen.findByRole('heading', { name: 'This link is no longer valid' })).toBeInTheDocument()
  })

  it('cannot be accepted once it is no longer open, and speaks the quote’s language', async () => {
    server.use(http.get('*/api/online/estimates/971', () => HttpResponse.json(quote({ language: 'nl', state: 'rejected', signable: false }))))
    renderAt('/online/estimates/971?code=123456789012')
    expect(await screen.findByRole('heading', { level: 1, name: 'Offerte 2026-0027' })).toBeInTheDocument()
    expect(screen.getByText('Deze offerte kan niet meer online worden geaccepteerd.')).toBeInTheDocument()
    expect(screen.queryByRole('form')).toBeNull()
  })
})
