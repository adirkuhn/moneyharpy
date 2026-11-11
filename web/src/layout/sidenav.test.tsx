import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

function open(admin = harpy) {
  server.use(
    signedIn(),
    listing([admin]),
    http.get('*/api/v2/administrations/11', () => HttpResponse.json(admin)),
    http.get('*/api/v2/11/overview', () => HttpResponse.json({ recently_processed: [] })),
    http.get('*/api/v2/11/financial_mutations', () => HttpResponse.json([])),
    http.get('*/api/v2/11/financial_accounts', () => HttpResponse.json([])),
  )
}

async function menu() {
  return screen.findByRole('navigation', { name: 'Administration menu' })
}

describe('side menu', () => {
  it('lists every part of the administration in order', async () => {
    open()
    renderAt('/11/feed')
    const nav = await menu()
    const top = within(nav).getAllByRole('listitem').filter((li) => li.parentElement?.classList.contains('sidenav__items'))
    const names = top.map((li) => li.querySelector('.sidenav__link')?.textContent)
    expect(names).toEqual([
      'Overview', 'Invoices', 'Incoming', 'Quotes', 'Bank', 'Timesheets', 'Contacts', 'VAT returns',
      'Reports', 'Products', 'Assets', 'Projects', 'Task lists', 'Settings',
    ])
  })

  it('marks the page that is open', async () => {
    open()
    renderAt('/11/feed')
    const nav = await menu()
    expect(within(nav).getByRole('link', { name: 'Overview' })).toHaveAttribute('aria-current', 'page')
    expect(within(nav).getByRole('link', { name: 'Overview' })).toHaveAttribute('href', '/11/feed')
    expect(within(nav).getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/11/settings')
  })

  it('opens the section of the page that is open', async () => {
    open()
    renderAt('/11/financial_mutations')
    const nav = await menu()
    expect(within(nav).getByRole('button', { name: 'Bank' })).toHaveAttribute('aria-expanded', 'true')
    expect(within(nav).getByRole('link', { name: 'Transactions' })).toHaveAttribute('aria-current', 'page')
    expect(within(nav).getByRole('link', { name: 'Financial accounts' })).toHaveAttribute('href', '/11/settings/bank')
    expect(within(nav).getByRole('button', { name: 'Reports' })).toHaveAttribute('aria-expanded', 'false')
  })

  it('expands a section on request', async () => {
    open()
    renderAt('/11/feed')
    const nav = await menu()
    const reports = within(nav).getByRole('button', { name: 'Reports' })
    expect(within(nav).queryByRole('link', { name: 'Balance sheet' })).toBeNull()
    await userEvent.click(reports)
    expect(reports).toHaveAttribute('aria-expanded', 'true')
    const list = document.getElementById(reports.getAttribute('aria-controls') ?? '')!
    expect(within(list).getAllByRole('link')).toHaveLength(18)
    await userEvent.click(reports)
    expect(within(nav).queryByRole('link', { name: 'Balance sheet' })).toBeNull()
  })

  it('shows parts that do not exist yet without leading anywhere', async () => {
    open()
    renderAt('/11/feed')
    const nav = await menu()
    const quotes = within(nav).getByRole('link', { name: 'Quotes' })
    expect(quotes).toHaveAttribute('aria-disabled', 'true')
    expect(quotes).not.toHaveAttribute('href')
    expect(quotes).toHaveAccessibleDescription('Not available yet')
    await userEvent.click(within(nav).getByRole('button', { name: 'Bank' }))
    expect(within(nav).getByRole('link', { name: 'Payments' })).toHaveAttribute('aria-disabled', 'true')
    expect(within(nav).getByRole('link', { name: 'Transactions' })).not.toHaveAttribute('aria-disabled')
  })

  it('leads to the sales and external invoices', async () => {
    open()
    renderAt('/11/feed')
    const nav = await menu()
    await userEvent.click(within(nav).getByRole('button', { name: 'Invoices' }))
    expect(within(nav).getByRole('link', { name: 'Sales invoices' })).toHaveAttribute('href', '/11/sales_invoices')
    expect(within(nav).getByRole('link', { name: 'External invoices' })).toHaveAttribute('href', '/11/external_sales_invoices')
    expect(within(nav).getByRole('link', { name: 'Recurring invoices' })).toHaveAttribute('aria-disabled', 'true')
  })

  it('shows only the parts a member may use', async () => {
    open(administration({ id: '11', user_type: 'employee', permissions: ['time_entries'] }))
    renderAt('/11/feed')
    const nav = await menu()
    expect(within(nav).getByRole('button', { name: 'Timesheets' })).toBeInTheDocument()
    expect(within(nav).getByRole('link', { name: 'Projects' })).toBeInTheDocument()
    expect(within(nav).queryByRole('button', { name: 'Bank' })).toBeNull()
    expect(within(nav).queryByRole('button', { name: 'Invoices' })).toBeNull()
    expect(within(nav).queryByRole('link', { name: 'Incoming' })).toBeNull()
    expect(within(nav).getByRole('link', { name: 'Overview' })).toBeInTheDocument()
  })

  it('shows the bank to a member with only that permission', async () => {
    open(administration({ id: '11', user_type: 'employee', permissions: ['bank'] }))
    renderAt('/11/feed')
    const nav = await menu()
    expect(within(nav).getByRole('button', { name: 'Bank' })).toBeInTheDocument()
    expect(within(nav).getByRole('button', { name: 'Reports' })).toBeInTheDocument()
    expect(within(nav).queryByRole('button', { name: 'Invoices' })).toBeNull()
  })

  it('collapses to icons and remembers it', async () => {
    open()
    renderAt('/11/feed')
    const nav = await menu()
    await userEvent.click(within(nav).getByRole('button', { name: 'Collapse menu' }))
    expect(nav).toHaveClass('is-collapsed')
    expect(within(nav).getByRole('link', { name: 'Overview' })).toHaveAttribute('title', 'Overview')
    expect(localStorage.getItem('moneyharpy.sidenav')).toBe('collapsed')
    await userEvent.click(within(nav).getByRole('button', { name: 'Expand menu' }))
    expect(nav).not.toHaveClass('is-collapsed')
    expect(localStorage.getItem('moneyharpy.sidenav')).toBe('expanded')
  })

  it('expands again to show the parts of a section', async () => {
    localStorage.setItem('moneyharpy.sidenav', 'collapsed')
    open()
    renderAt('/11/feed')
    const nav = await menu()
    expect(nav).toHaveClass('is-collapsed')
    await userEvent.click(within(nav).getByRole('button', { name: 'Reports' }))
    expect(nav).not.toHaveClass('is-collapsed')
    expect(within(nav).getByRole('link', { name: 'Balance sheet' })).toBeInTheDocument()
  })

  it('opens as a drawer from the top bar on small screens', async () => {
    open()
    renderAt('/11/feed')
    const nav = await menu()
    const toggle = await screen.findByRole('button', { name: 'Menu' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveAttribute('aria-controls', nav.id)
    await userEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(nav).toHaveClass('is-open')
    await userEvent.keyboard('{Escape}')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await userEvent.click(toggle)
    await userEvent.click(within(nav).getByRole('button', { name: 'Bank' }))
    await userEvent.click(within(nav).getByRole('link', { name: 'Transactions' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/financial_mutations'))
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
  })

  it('leaves settings to their own menu', async () => {
    open()
    renderAt('/11/settings/company')
    expect(await screen.findByRole('navigation', { name: 'Settings' })).toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: 'Administration menu' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Menu' })).toBeNull()
  })

  it('is not shown outside an administration', async () => {
    open()
    renderAt('/administrations')
    await screen.findByRole('heading', { level: 1 })
    expect(screen.queryByRole('navigation', { name: 'Administration menu' })).toBeNull()
  })

  it('replaces the section links in the top bar', async () => {
    open()
    renderAt('/11/feed')
    await menu()
    const top = screen.getByRole('navigation', { name: 'Main' })
    expect(within(top).queryByRole('link', { name: 'Overview' })).toBeNull()
    expect(within(top).queryByRole('link', { name: 'Settings' })).toBeNull()
  })
})
