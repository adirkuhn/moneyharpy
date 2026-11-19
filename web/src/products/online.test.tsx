import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

function loaded() {
  return [signedIn(), listing([harpy]), http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy))]
}

describe('online sales', () => {
  it('introduces selling online, with Add for a payment link or a subscription template, as Moneybird', async () => {
    server.use(...loaded())
    renderAt('/11/payment_links')
    expect(await screen.findByRole('heading', { name: 'Online sales', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Sell directly from MoneyHarpy' })).toBeInTheDocument()
    const nav = await screen.findByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Online sales', current: 'page' })).toHaveAttribute('href', '/11/payment_links')

    await userEvent.click(screen.getByRole('button', { name: 'Add' }))
    expect(screen.getByRole('link', { name: 'Subscription template' })).toHaveAttribute('href', '/11/subscription_templates/new')
    await userEvent.click(screen.getByRole('link', { name: 'Payment link' }))
    expect(await screen.findByRole('heading', { name: 'Not ready for online sales' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/11/payment_links/incomplete')
    expect(screen.getByRole('list', { name: 'What online sales need' })).toHaveTextContent(/Online payment methods.*A workflow with online payment methods/)
    // As Moneybird's, the way back is both beside the title and under the card.
    expect(screen.getAllByRole('link', { name: 'Back to online sales' })).toHaveLength(2)
    await userEvent.click(screen.getAllByRole('link', { name: 'Back to online sales' })[1]!)
    expect(screen.getByTestId('path')).toHaveTextContent(/^\/11\/payment_links$/)
  })

  it('answers a product\'s payment link with the same, and leads back to the product', async () => {
    server.use(...loaded())
    renderAt('/11/payment_links/501/edit?return_to=%2F11%2Fproducts%2F501')
    expect(await screen.findByRole('heading', { name: 'Not ready for online sales' })).toBeInTheDocument()
    for (const back of screen.getAllByRole('link', { name: 'Back to the product' })) {
      expect(back).toHaveAttribute('href', '/11/products/501')
    }
  })
})
