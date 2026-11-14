import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import type { RegistryDuplicate } from './queries'
import { contact, loaded } from './testkit'

const kvkLabel = 'Netherlands Chamber of Commerce KVK number'

/** Every region code the browser has a name for, as the server lists all ISO countries. */
const allCountries = (() => {
  const names = new Intl.DisplayNames(['en'], { type: 'region', fallback: 'none' })
  const letters = [...'ABCDEFGHIJKLMNOPQRSTUVWXYZ']
  return letters.flatMap((a) => letters.map((b) => a + b)).filter((code) => {
    try {
      return names.of(code) !== undefined
    } catch {
      return false
    }
  })
})()

/** Picks a country in the Address section's searchable select. */
async function chooseCountry(name: string) {
  await userEvent.click(screen.getByRole('combobox', { name: 'Country' }))
  await userEvent.type(screen.getByRole('searchbox', { name: 'Find country' }), name)
  await userEvent.click(screen.getByRole('option', { name }))
}

describe('the registry numbers of a contact', () => {
  type Sent = { contact?: Record<string, unknown> }
  const creating = (sent: Sent) => http.post('*/api/v2/11/contacts', async ({ request }) => {
    Object.assign(sent, await request.json())
    return HttpResponse.json(contact({ id: '903', uri: '/11/contacts/903' }), { status: 201 })
  })
  const shop = (field: RegistryDuplicate['field']): RegistryDuplicate => ({
    field, contact_id: '901', contact_name: 'Example Shop B.V.', message: '',
  })

  describe('duplicates', () => {
    it('warns on leaving the KVK number that another contact has it, links to it and refuses to save', async () => {
      const sent: Sent = {}
      const asked: string[] = []
      server.use(
        http.get('*/api/v2/11/contacts/registry_duplicates', ({ request }) => {
          asked.push(new URL(request.url).searchParams.get('chamber_of_commerce') ?? '')
          return HttpResponse.json([shop('chamber_of_commerce')])
        }),
        ...loaded(), creating(sent),
      )
      renderAt('/11/contacts/new')
      await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop Amsterdam')
      await userEvent.type(screen.getByLabelText(kvkLabel), '1234 5678')
      await userEvent.tab()
      const warning = await screen.findByRole('alert')
      expect(warning).toHaveTextContent('A contact with this KVK number already exists: Example Shop B.V.')
      expect(within(warning).getByRole('link', { name: 'Open contact' })).toHaveAttribute('href', '/11/contacts/901')
      expect(asked[0]).toBe('1234 5678')

      await userEvent.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(asked).toHaveLength(2))
      expect(sent.contact).toBeUndefined()
      expect(screen.getByTestId('path')).toHaveTextContent('/11/contacts/new')
    })

    it('warns about a VAT number another contact has, and forgets it once the number changes', async () => {
      server.use(...loaded(undefined, undefined, { duplicates: [shop('tax_number')] }))
      renderAt('/11/contacts/new')
      await userEvent.type(await screen.findByLabelText('VAT-id'), 'NL123456782B01')
      await userEvent.tab()
      expect(await screen.findByRole('alert')).toHaveTextContent('A contact with this VAT number already exists: Example Shop B.V.')
      await userEvent.type(screen.getByLabelText('VAT-id'), '9')
      expect(screen.queryByRole('alert')).toBeNull()
    })

    it('saves a contact whose numbers no one else has', async () => {
      const sent: Sent = {}
      server.use(...loaded(), creating(sent))
      renderAt('/11/contacts/new')
      await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop B.V.')
      await userEvent.type(screen.getByLabelText(kvkLabel), '12345678')
      await userEvent.tab()
      await userEvent.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(sent.contact).toMatchObject({ chamber_of_commerce: '12345678' }))
      expect(screen.queryByRole('alert')).toBeNull()
    })

    it('leaves the contact itself out when editing', async () => {
      const asked: string[] = []
      server.use(http.get('*/api/v2/11/contacts/registry_duplicates', ({ request }) => {
        asked.push(new URL(request.url).searchParams.get('except_id') ?? '')
        return HttpResponse.json([])
      }), ...loaded())
      renderAt('/11/contacts/901/edit')
      await userEvent.type(await screen.findByLabelText(kvkLabel), '87654321')
      await userEvent.tab()
      await waitFor(() => expect(asked).toEqual(['901']))
    })
  })

  describe('the KVK lookup', () => {
    const found = { name: 'Example Shop B.V.', chamber_of_commerce: '12345678', address1: 'Dam 1', zipcode: '1012JS', city: 'Amsterdam' }
    const kvk = (answer: () => Response, asked: string[] = []) => http.get('*/api/v2/11/kvk_lookup', ({ request }) => {
      asked.push(new URL(request.url).searchParams.get('kvk_number') ?? '')
      return answer()
    })

    it('fills in only the empty fields when leaving a KVK number, and says so', async () => {
      const asked: string[] = []
      server.use(...loaded(undefined, undefined, { companySearch: true }), kvk(() => HttpResponse.json(found), asked))
      renderAt('/11/contacts/new')
      await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop')
      await userEvent.type(screen.getByLabelText('City'), 'Utrecht')
      await userEvent.type(screen.getByLabelText(kvkLabel), '1234 5678')
      await userEvent.tab()
      expect(await screen.findByText('Filled in from the KVK register')).toBeInTheDocument()
      expect(asked).toEqual(['12345678'])
      expect(screen.getByLabelText('Company name')).toHaveValue('Example Shop')
      expect(screen.getByRole('textbox', { name: 'Address' })).toHaveValue('Dam 1')
      expect(screen.getByLabelText('Zipcode')).toHaveValue('1012JS')
      expect(screen.getByLabelText('City')).toHaveValue('Utrecht')
    })

    it('looks up on Look up too', async () => {
      server.use(...loaded(undefined, undefined, { companySearch: true }), kvk(() => HttpResponse.json(found)))
      renderAt('/11/contacts/new')
      await userEvent.type(await screen.findByLabelText(kvkLabel), '12345678')
      await userEvent.click(screen.getByRole('button', { name: 'Look up' }))
      expect(await screen.findByText('Filled in from the KVK register')).toBeInTheDocument()
      expect(screen.getByLabelText('Company name')).toHaveValue('Example Shop B.V.')
    })

    it('says quietly when KVK does not know the number or cannot be reached', async () => {
      let answer = () => HttpResponse.json({ error: 'Not found' }, { status: 404 })
      server.use(...loaded(undefined, undefined, { companySearch: true }), kvk(() => answer()))
      renderAt('/11/contacts/new')
      await userEvent.type(await screen.findByLabelText(kvkLabel), '12345678')
      await userEvent.tab()
      expect(await screen.findByText('Not found in KVK')).toBeInTheDocument()
      expect(screen.getByLabelText('Company name')).toHaveValue('')

      answer = () => HttpResponse.json({ error: 'KVK search is unavailable right now', code: 'kvk_unavailable' }, { status: 503 })
      await userEvent.click(screen.getByRole('button', { name: 'Look up' }))
      expect(await screen.findByText('KVK is not available right now')).toBeInTheDocument()
    })

    it('asks nothing while KVK is off, or for a number without eight digits', async () => {
      const asked: string[] = []
      server.use(...loaded(), kvk(() => HttpResponse.json(found), asked))
      renderAt('/11/contacts/new')
      await userEvent.type(await screen.findByLabelText(kvkLabel), '12345678')
      await userEvent.tab()
      expect(screen.queryByRole('button', { name: 'Look up' })).toBeNull()
      expect(asked).toEqual([])
    })
  })

  it('tells a Dutch KVK number without eight digits and does not save it', async () => {
    const sent: Sent = {}
    server.use(...loaded(), creating(sent))
    renderAt('/11/contacts/new')
    await userEvent.type(await screen.findByLabelText('Company name'), 'Example Shop B.V.')
    await userEvent.type(screen.getByLabelText(kvkLabel), '1234')
    await userEvent.tab()
    expect(screen.getByText('A KVK number has 8 digits')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(sent.contact).toBeUndefined()
  })

  describe('contacts in other countries', () => {
    it('offers every country, searchable and with a decorative flag', async () => {
      server.use(...loaded(undefined, undefined, { countries: allCountries }))
      renderAt('/11/contacts/new')
      await userEvent.click(await screen.findByRole('combobox', { name: 'Country' }))
      const list = screen.getByRole('listbox', { name: 'Country' })
      expect(within(list).getAllByRole('option').length).toBeGreaterThan(200)
      // The flag is hidden from screen readers, so the option is named by the country alone.
      const belgium = within(list).getByRole('option', { name: 'Belgium' })
      expect(belgium.querySelector('[aria-hidden="true"]')).toHaveTextContent('🇧🇪')
      await userEvent.type(screen.getByRole('searchbox', { name: 'Find country' }), 'united st')
      expect(within(list).getByRole('option', { name: 'United States' })).toBeInTheDocument()
    })

    it.each([
      ['Belgium', 'Enterprise number (KBO/BCE)'],
      ['Germany', 'Commercial register number (Handelsregister)'],
      ['Portugal', 'NIPC'],
      ['France', 'SIREN/SIRET'],
      ['Spain', 'CIF/NIF'],
      ['United States', 'Company registration number'],
    ])('names the number after the register of %s, without the KVK format', async (country, label) => {
      const sent: Sent = {}
      server.use(...loaded(undefined, undefined, { countries: ['BE', 'DE', 'ES', 'FR', 'NL', 'PT', 'US'] }), creating(sent))
      renderAt('/11/contacts/new')
      expect(await screen.findByLabelText(kvkLabel)).toBeInTheDocument()
      await chooseCountry(country)
      await userEvent.type(screen.getByLabelText('Company name'), 'Example Shop')
      await userEvent.type(screen.getByLabelText(label), 'B 1234')
      await userEvent.tab()
      expect(screen.queryByText('A KVK number has 8 digits')).toBeNull()
      await userEvent.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(sent.contact).toMatchObject({ chamber_of_commerce: 'B 1234' }))
    })

    it('offers KVK search and lookup only for a Dutch contact', async () => {
      const searched: string[] = []
      server.use(...loaded(undefined, undefined, { companySearch: true }),
        http.get('*/api/onboarding/company_search', ({ request }) => {
          searched.push(new URL(request.url).searchParams.get('query') ?? '')
          return HttpResponse.json([])
        }))
      renderAt('/11/contacts/new')
      const country = await screen.findByRole('combobox', { name: 'Country of the company' })
      // Every country, not only the ones with a register.
      expect(within(country).getAllByRole('option').map((o) => o.textContent)).toEqual(['Belgium', 'Germany', 'Netherlands'])
      await userEvent.selectOptions(country, 'BE')
      expect(screen.getByRole('combobox', { name: 'Country' })).toHaveTextContent('Belgium')
      await userEvent.type(screen.getByLabelText('Company name'), 'Example Shop')
      expect(screen.getByLabelText('Company name')).not.toHaveAttribute('role', 'combobox')
      await userEvent.type(screen.getByLabelText('Enterprise number (KBO/BCE)'), '12345678')
      expect(screen.queryByRole('button', { name: 'Look up' })).toBeNull()
      await new Promise((resolve) => setTimeout(resolve, 400))
      expect(searched).toEqual([])
    })

    it('looks an EU VAT number up in VIES for a contact in any EU country', async () => {
      const asked: string[] = []
      server.use(...loaded(undefined, undefined, { vatLookup: true }), http.get('*/api/v2/11/vat_lookup', ({ request }) => {
        asked.push(new URL(request.url).searchParams.get('vat_number') ?? '')
        return HttpResponse.json({ valid: true, name: 'EXAMPLE SHOP BV', address1: 'VOORBEELDSTRAAT 1', zipcode: '1000', city: 'BRUSSEL', country: 'BE' })
      }))
      renderAt('/11/contacts/new')
      await screen.findByLabelText('VAT-id')
      await chooseCountry('Germany')
      await userEvent.type(screen.getByLabelText('VAT-id'), 'BE0123456789')
      await userEvent.tab()
      expect(await screen.findByText('Filled in from the EU VIES register')).toBeInTheDocument()
      expect(asked).toEqual(['BE0123456789'])
      expect(screen.getByLabelText('Company name')).toHaveValue('EXAMPLE SHOP BV')
    })
  })
})
