import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { ann, contact, loaded, rowsOf } from './testkit'

describe('contacts list', () => {
  it('lists contacts by name, as Moneybird does', async () => {
    server.use(...loaded([contact({ firstname: 'Ann', lastname: 'de Vries' }), ann]))
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['', 'Name'])
    expect(within(table).getByRole('link', { name: 'Example Shop B.V. — Ann de Vries' })).toHaveAttribute('href', '/11/contacts/901')
    expect(within(table).getByRole('link', { name: 'Ann Jansen' })).toHaveAttribute('href', '/11/contacts/902')
    expect(screen.getByRole('button', { name: 'Contacts' })).toBeInTheDocument()
    expect(screen.getByRole('searchbox', { name: 'Search for contacts or companies' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Export' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Import' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add' })).toHaveAttribute('href', '/11/contacts/new')
    const nav = screen.getByRole('navigation', { name: 'Administration menu' })
    expect(within(nav).getByRole('link', { name: 'Contacts' })).toHaveAttribute('aria-current', 'page')
  })

  it('shows the person of a company in grey after its name', async () => {
    server.use(...loaded([contact({ firstname: 'Ann', lastname: 'de Vries' })]))
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    const person = within(table).getByText('Ann de Vries')
    expect(person).toHaveClass('mute')
    expect(person.parentElement).toHaveTextContent(/^Example Shop B\.V\. — Ann de Vries$/)
  })

  it('groups the contacts by their first letter', async () => {
    server.use(...loaded([
      contact({ id: '1', company_name: '- shop' }),
      contact({ id: '2', company_name: '123 Repairs' }),
      contact({ id: '3', company_name: '123 Repairs West' }),
      contact({ id: '4', company_name: '8wraps' }),
      contact({ id: '5', company_name: 'Acme' }),
      contact({ id: '6', company_name: '', firstname: 'Ann', lastname: 'Jansen' }),
      contact({ id: '7', company_name: 'Één Bakery' }),
      contact({ id: '8', company_name: 'example shop' }),
    ]))
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    expect(rowsOf(table).map((cells) => cells[0])).toEqual(['-', '1', '', '8', 'A', '', 'E', ''])
  })

  it('searches as the user types', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/contacts')
    await userEvent.type(await screen.findByRole('searchbox', { name: 'Search for contacts or companies' }), 'shop')
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('query')).toBe('shop'))
    expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts')
  })

  it("offers Moneybird's quick filters", async () => {
    server.use(...loaded())
    renderAt('/11/contacts?filter=trusted_type:trusted')
    await userEvent.click(await screen.findByRole('button', { name: 'Trusted contacts' }))
    const panel = screen.getByRole('search', { name: 'Contact filters' })
    expect(within(panel).getByRole('link', { name: 'Trusted contacts' })).toHaveAttribute('aria-current', 'true')
    expect(within(panel).getByRole('link', { name: 'All contacts' })).toHaveAttribute('href', '/11/contacts')
    expect(within(panel).getByRole('link', { name: 'Contacts with delivery method manual' })).toHaveAttribute('href', '/11/contacts?filter=delivery_method%3Amanual')
    expect(within(panel).getByRole('link', { name: 'Contacts with delivery method e-mail' })).toHaveAttribute('href', '/11/contacts?filter=delivery_method%3Aemail')
  })

  it('filters on type, trust, workflows, mandate and delivery method', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen))
    renderAt('/11/contacts')
    await screen.findByRole('table', { name: 'Contacts' })
    await userEvent.click(screen.getByRole('button', { name: 'Contacts' }))
    const panel = screen.getByRole('search', { name: 'Contact filters' })
    const group = (name: string) => within(panel).getByRole('group', { name })
    expect(within(group('Trusted contact')).getAllByRole('radio').map((r) => r.closest('label')?.textContent)).toEqual(['All', 'Trusted', 'Not trusted'])
    await userEvent.click(within(group('Contact type')).getByRole('radio', { name: 'Company' }))
    await userEvent.click(within(group('Mandate')).getByRole('radio', { name: 'Manual mandate' }))
    await userEvent.click(within(group('Delivery method')).getByRole('radio', { name: 'Peppol' }))
    expect(within(panel).getByRole('combobox', { name: 'Estimate workflow' })).toHaveDisplayValue('All workflows')
    await userEvent.selectOptions(within(panel).getByRole('combobox', { name: 'Invoice workflow' }), 'Standard invoices')
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    await waitFor(() => expect(seen.at(-1)?.pathname).toBe('/api/v2/11/contacts/filter'))
    expect(seen.at(-1)?.searchParams.get('filter')).toBe('contact_type:company,invoice_workflow_id:601,mandate_type:manual,delivery_method:peppol')
    expect(screen.getByRole('button', { name: 'Filtered contacts' })).toBeInTheDocument()
  })

  it('counts the deactivated contacts under the list and shows them on request', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen, { archived: 3 }))
    renderAt('/11/contacts')
    expect(await screen.findByText('There are 3 deactivated items.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Show deactivated items.' }))
    await waitFor(() => expect(seen.some((u) => u.searchParams.get('include_archived') === 'true' && u.searchParams.get('per_page') === '50')).toBe(true))
    await userEvent.click(await screen.findByRole('button', { name: 'Hide deactivated items.' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Show deactivated items.' })).toBeInTheDocument())
  })

  it('says nothing about deactivated contacts when there are none', async () => {
    server.use(...loaded())
    renderAt('/11/contacts')
    await screen.findByRole('table', { name: 'Contacts' })
    expect(screen.queryByText(/deactivated/)).toBeNull()
  })

  it('points out possible duplicates and lists them', async () => {
    const twin = contact({ id: '903', uri: '/11/contacts/903' })
    server.use(...loaded(undefined, undefined, { doubles: [[contact(), twin]] }))
    renderAt('/11/contacts')
    const banner = await screen.findByRole('region', { name: '2 possible duplicate contacts found' })
    expect(within(banner).getByText('Merge them to process documents even more accurately.')).toBeInTheDocument()
    await userEvent.click(within(banner).getByRole('link', { name: 'View contacts' }))
    const table = await screen.findByRole('table', { name: 'Contacts' })
    await waitFor(() => expect(within(table).getAllByRole('link', { name: 'Example Shop B.V.' }).map((a) => a.getAttribute('href'))).toEqual(['/11/contacts/901', '/11/contacts/903']))
    expect(screen.getByRole('button', { name: 'Possible duplicates' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: /duplicate contacts found/ })).toBeNull()
  })

  it('opens a contact from anywhere in its row', async () => {
    server.use(...loaded())
    renderAt('/11/contacts')
    const table = await screen.findByRole('table', { name: 'Contacts' })
    await userEvent.click(within(table).getAllByRole('row')[2]!.querySelector('td')!)
    expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts/902')
  })

  it('pages through many contacts', async () => {
    const seen: URL[] = []
    server.use(...loaded(undefined, seen, { total: 120 }))
    renderAt('/11/contacts')
    const pages = await screen.findByRole('navigation', { name: 'Pagination' })
    expect(within(pages).getByText('1')).toHaveAttribute('aria-current', 'page')
    await userEvent.click(within(pages).getByRole('link', { name: 'Page 3' }))
    await waitFor(() => expect(seen.at(-1)?.searchParams.get('page')).toBe('3'))
    expect(within(screen.getByRole('navigation', { name: 'Pagination' })).getByText('3')).toHaveAttribute('aria-current', 'page')
  })

  it('invites adding the first contact', async () => {
    server.use(...loaded([]))
    renderAt('/11/contacts')
    expect(await screen.findByText('No contacts yet. Add your first customer or supplier.')).toBeInTheDocument()
  })
})
