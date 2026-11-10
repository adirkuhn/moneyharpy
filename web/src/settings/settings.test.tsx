import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, identity, listing, server, signedIn } from '../test/server'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const def = identity()
const sender = identity({ id: '501', company_name: 'Trade Name', default: false, phone: '0612345678' })

const slogan = { id: '701', administration_id: '11', name: 'Slogan', source: 'identity' as const, uri: '/11/custom_fields/701' }
const contactField = { id: '702', administration_id: '11', name: 'Contact ref', source: 'contact' as const, uri: '/11/custom_fields/702' }

const loaded = (identities = [def, sender]) => [
  signedIn(),
  listing([harpy]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
  http.get('*/api/v2/11/identities', () => HttpResponse.json(identities)),
  http.get('*/api/v2/11/identities/default', () => HttpResponse.json(def)),
  http.get('*/api/v2/11/identities/501', () => HttpResponse.json(sender)),
  http.get('*/api/v2/11/custom_fields', () => HttpResponse.json([slogan, contactField])),
  http.get('*/api/onboarding/options', () =>
    HttpResponse.json({ countries: ['BE', 'NL'], currencies: ['EUR', 'USD'], popular_currencies: ['EUR'], company_search_countries: [], address_lookup_countries: [] }),
  ),
]

describe('the settings area', () => {
  it('opens company settings by default', async () => {
    server.use(...loaded())
    renderAt('/11/settings')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/company'))
    expect(await screen.findByRole('heading', { name: 'Company', level: 1 })).toBeInTheDocument()
  })

  it('lists every section and leads back to the administration', async () => {
    server.use(...loaded())
    renderAt('/11/settings/company')
    const nav = await screen.findByRole('navigation', { name: 'Settings' })
    for (const [name, href] of [
      ['Back', '/11'],
      ['Company', '/11/settings/company'],
      ['Invoices and estimates', '/11/settings/invoices_and_estimates'],
      ['Bookkeeping', '/11/settings/bookkeeping'],
      ['Bank', '/11/settings/bank'],
      ['Management', '/11/settings/administration'],
      ['Integrations', '/11/settings/connections'],
    ] as const) {
      expect(within(nav).getByRole('link', { name })).toHaveAttribute('href', href)
    }
    expect(within(nav).getByRole('link', { name: 'Company' })).toHaveAttribute('aria-current', 'page')
  })

  it('lists the parts of the open section', async () => {
    server.use(...loaded())
    renderAt('/11/settings/company')
    const nav = await screen.findByRole('navigation', { name: 'Settings' })
    expect(within(nav).getByRole('link', { name: 'Company details' })).toHaveAttribute('href', '/11/settings/company#company-details')
    expect(within(nav).getByRole('link', { name: 'Sender addresses' })).toHaveAttribute('href', '/11/settings/company#sender-addresses')
    expect(await screen.findByRole('region', { name: 'Company details' })).toHaveAttribute('id', 'company-details')
    expect(screen.getByRole('region', { name: 'Sender addresses' })).toHaveAttribute('id', 'sender-addresses')
  })

  it('keeps sections without a feature as clear placeholders', async () => {
    server.use(...loaded())
    renderAt('/11/settings/bank')
    expect(await screen.findByRole('heading', { name: 'Bank', level: 1 })).toBeInTheDocument()
    expect(screen.getByText('This section is not available yet.')).toBeInTheDocument()
  })

  it('reports an administration the user cannot open', async () => {
    server.use(signedIn(), listing([]), http.get('*/api/v2/administrations/99', () => HttpResponse.json({ error: 'x' }, { status: 404 })))
    renderAt('/99/settings/company')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: 'Settings' })).toBeNull()
  })
})

describe('company settings', () => {
  it('shows the company details with an edit link', async () => {
    server.use(...loaded())
    renderAt('/11/settings/company')
    const card = await screen.findByRole('region', { name: 'Company details' })
    expect(await within(card).findByText('Dam 1')).toBeInTheDocument()
    expect(within(card).getByText('1012JS Amsterdam')).toBeInTheDocument()
    expect(within(card).getByText('Netherlands')).toBeInTheDocument()
    expect(within(card).getByText('12345678')).toBeInTheDocument()
    expect(within(card).getByText('NL91 ABNA 0417 1643 00')).toBeInTheDocument()
    expect(within(card).getByRole('link', { name: 'Edit' })).toHaveAttribute('href', '/11/identities/edit_default')
  })

  it('marks empty details', async () => {
    server.use(...loaded())
    renderAt('/11/settings/company')
    const card = await screen.findByRole('region', { name: 'Company details' })
    await within(card).findByText('Dam 1')
    expect(within(card).getAllByText('-').length).toBeGreaterThan(0)
    expect(within(card).getAllByText('Not set').length).toBeGreaterThan(0)
  })

  it('lists sender addresses and offers to add one', async () => {
    server.use(...loaded())
    renderAt('/11/settings/company')
    const block = await screen.findByRole('region', { name: 'Sender addresses' })
    expect(await within(block).findByText('Trade Name')).toBeInTheDocument()
    expect(within(block).getByRole('link', { name: 'Add' })).toHaveAttribute('href', '/11/identities/new')
    expect(within(block).getByRole('link', { name: 'Edit Trade Name' })).toHaveAttribute('href', '/11/identities/501/edit')
  })

  it('says when there are no sender addresses', async () => {
    server.use(...loaded([def]))
    renderAt('/11/settings/company')
    expect(await screen.findByText('Add an address to send invoices under another name.')).toBeInTheDocument()
  })
})

describe('editing identities', () => {
  it('updates the company details and returns to settings', async () => {
    let sent: { identity?: Record<string, string> } = {}
    server.use(
      ...loaded(),
      http.patch('*/api/v2/11/identities/default', async ({ request }) => {
        sent = (await request.json()) as typeof sent
        return HttpResponse.json(def)
      }),
      http.patch('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    )
    renderAt('/11/identities/edit_default')
    const phone = await screen.findByLabelText('Phone number')
    await userEvent.type(phone, '0201234567')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/company'))
    expect(sent.identity).toMatchObject({ phone: '0201234567', company_name: 'Harpy B.V.', address1: 'Dam 1' })
  })

  it('shows what is wrong', async () => {
    server.use(
      ...loaded(),
      http.patch('*/api/v2/11/identities/default', () =>
        HttpResponse.json({ error: { bank_account_number: ['is not a valid IBAN'] } }, { status: 422 }),
      ),
    )
    renderAt('/11/identities/edit_default')
    const account = await screen.findByLabelText('Account number')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(account).toHaveAccessibleDescription('Account number is not a valid IBAN. Check it for typos; an IBAN starts with a country code, like NL91 ABNA 0417 1643 00.'),
    )
    expect(account).toHaveAttribute('aria-invalid', 'true')
  })

  it('edits beside the settings menu with a way back to Company', async () => {
    server.use(...loaded())
    renderAt('/11/identities/edit_default')
    expect(await screen.findByRole('heading', { name: 'Edit company information', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'To Company' })).toHaveAttribute('href', '/11/settings/company')
    expect(screen.getByRole('note')).toHaveTextContent('The company details are shown on your invoices and estimates.')
    const nav = screen.getByRole('navigation', { name: 'Settings' })
    expect(within(nav).getByRole('link', { name: 'Company' })).toHaveAttribute('aria-current', 'true')
  })

  it('keeps the second address line behind a button', async () => {
    server.use(...loaded())
    renderAt('/11/identities/edit_default')
    await screen.findByLabelText('Address')
    expect(screen.queryByLabelText('Additional address details')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Add address details' }))
    await userEvent.type(screen.getByLabelText('Additional address details'), 'Floor 2')
    await userEvent.click(screen.getByRole('button', { name: 'Remove address details' }))
    expect(screen.queryByLabelText('Additional address details')).toBeNull()
    expect(screen.getByRole('button', { name: 'Add address details' })).toBeInTheDocument()
  })

  it('explains a missing VAT number', async () => {
    let sent: { identity?: Record<string, unknown> } = {}
    server.use(
      ...loaded(),
      http.patch('*/api/v2/11/identities/default', async ({ request }) => {
        sent = (await request.json()) as typeof sent
        return HttpResponse.json(def)
      }),
      http.patch('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    )
    renderAt('/11/identities/edit_default')
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Company does not have a VAT number' }))
    await userEvent.selectOptions(screen.getByLabelText('Reason'), 'industry_exemption')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.identity).toMatchObject({ tax_number: '', tax_number_absence_reason: 'industry_exemption' }))
  })

  it('hides the missing VAT choice once a VAT number is typed', async () => {
    server.use(...loaded())
    renderAt('/11/identities/edit_default')
    await userEvent.type(await screen.findByLabelText('VAT number'), 'NL000099998B57')
    expect(screen.queryByRole('checkbox', { name: 'Company does not have a VAT number' })).toBeNull()
  })

  it('saves what the company does with its details', async () => {
    let sent: unknown
    server.use(
      ...loaded(),
      http.patch('*/api/v2/11/identities/default', () => HttpResponse.json(def)),
      http.patch('*/api/v2/administrations/11', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(harpy)
      }),
    )
    renderAt('/11/identities/edit_default')
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Company has been registered for the small business scheme (KOR)' }))
    await userEvent.type(screen.getByLabelText('Company activities'), 'Bakery')
    await userEvent.type(screen.getByLabelText('Website'), 'https://example.com')
    expect(screen.queryByRole('checkbox', { name: 'Company does not have a website' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/company'))
    expect(sent).toEqual({
      administration: {
        small_business_scheme: true,
        company_activities_description: 'Bakery',
        company_website_url: 'https://example.com',
        company_website_absent: false,
      },
    })
  })

  it('shows a website problem beside the website', async () => {
    server.use(
      ...loaded(),
      http.patch('*/api/v2/11/identities/default', () => HttpResponse.json(def)),
      http.patch('*/api/v2/administrations/11', () =>
        HttpResponse.json({ error: { company_website_url: ['is not a valid website address'] } }, { status: 422 }),
      ),
    )
    renderAt('/11/identities/edit_default')
    const website = await screen.findByLabelText('Website')
    await userEvent.type(website, 'nope')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(website).toHaveAccessibleDescription('Website is not a valid website address. Start it with https://, like https://example.com.'))
    expect(screen.getByTestId('path')).toHaveTextContent('/11/identities/edit_default')
  })

  it('fills in custom fields for identities', async () => {
    let sent: { identity?: Record<string, unknown> } = {}
    server.use(...loaded())
    server.use(
      http.get('*/api/v2/11/identities/default', () =>
        HttpResponse.json({ ...def, custom_fields: [{ id: '701', name: 'Slogan', value: 'Fresh' }] }),
      ),
      http.patch('*/api/v2/11/identities/default', async ({ request }) => {
        sent = (await request.json()) as typeof sent
        return HttpResponse.json(def)
      }),
      http.patch('*/api/v2/administrations/11', () => HttpResponse.json(harpy)),
    )
    renderAt('/11/identities/edit_default')
    const field = await screen.findByLabelText('Slogan')
    expect(field).toHaveValue('Fresh')
    expect(screen.queryByLabelText('Contact ref')).toBeNull()
    expect(screen.getByRole('link', { name: 'Add custom field' })).toHaveAttribute(
      'href',
      '/11/custom_fields/new?source=identity&return_to=%2F11%2Fidentities%2Fedit_default',
    )
    await userEvent.clear(field)
    await userEvent.type(field, 'Fast')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent.identity?.custom_fields_attributes).toEqual([{ id: '701', value: 'Fast' }]))
  })

  it('leaves company-wide details off sender addresses', async () => {
    server.use(...loaded())
    renderAt('/11/identities/new')
    await screen.findByLabelText('Company name')
    expect(screen.queryByLabelText('Website')).toBeNull()
    expect(screen.queryByRole('checkbox', { name: /small business scheme/ })).toBeNull()
    expect(screen.getByRole('heading', { name: 'New sender address', level: 1 })).toBeInTheDocument()
  })

  it('does not offer deleting the default identity', async () => {
    server.use(...loaded())
    renderAt('/11/identities/edit_default')
    await screen.findByLabelText('Company name')
    expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull()
  })

  it('adds a sender address', async () => {
    let sent: { identity?: Record<string, string> } = {}
    server.use(
      ...loaded(),
      http.post('*/api/v2/11/identities', async ({ request }) => {
        sent = (await request.json()) as typeof sent
        return HttpResponse.json(sender, { status: 201 })
      }),
    )
    renderAt('/11/identities/new')
    await userEvent.type(await screen.findByLabelText('Company name'), 'New Trade')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/company'))
    expect(sent.identity).toMatchObject({ company_name: 'New Trade', country: 'NL' })
  })

  it('asks for confirmation before deleting', async () => {
    let deleted = false
    server.use(
      ...loaded(),
      http.delete('*/api/v2/11/identities/501', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/identities/501/edit')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    expect(deleted).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: 'Confirm delete' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/company'))
    expect(deleted).toBe(true)
  })

  it('opens an identity by its URI', async () => {
    server.use(...loaded())
    renderAt('/11/identities/501')
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/identities/501/edit'))
  })

  it('reports an identity that does not exist', async () => {
    server.use(...loaded(), http.get('*/api/v2/11/identities/777', () => HttpResponse.json({ error: 'x' }, { status: 404 })))
    renderAt('/11/identities/777/edit')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })
})

describe('custom fields', () => {
  it('renames a custom field', async () => {
    let sent: unknown
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/custom_fields/701', () => HttpResponse.json(slogan)),
      http.patch('*/api/v2/11/custom_fields/701', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json({ ...slogan, name: 'Motto' })
      }),
    )
    renderAt('/11/custom_fields/701/edit?return_to=%2F11%2Fsettings%2Finvoices_and_estimates')
    expect(await screen.findByRole('heading', { name: 'Edit custom field', level: 1 })).toBeInTheDocument()
    expect(screen.getByLabelText('Used for')).toBeDisabled()
    const name = screen.getByLabelText('Name')
    await userEvent.clear(name)
    await userEvent.type(name, 'Motto')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/invoices_and_estimates'))
    expect(sent).toEqual({ custom_field: { name: 'Motto' } })
  })

  it('deletes a custom field after confirming', async () => {
    let deleted = false
    server.use(
      ...loaded(),
      http.get('*/api/v2/11/custom_fields/701', () => HttpResponse.json(slogan)),
      http.delete('*/api/v2/11/custom_fields/701', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/11/custom_fields/701/edit')
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    expect(deleted).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: 'Confirm delete' }))
    await waitFor(() => expect(deleted).toBe(true))
  })

  it('adds a custom field and returns to the form it came from', async () => {
    let sent: unknown
    server.use(
      ...loaded(),
      http.post('*/api/v2/11/custom_fields', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(slogan, { status: 201 })
      }),
    )
    renderAt('/11/custom_fields/new?source=identity&return_to=%2F11%2Fidentities%2Fedit_default')
    expect(await screen.findByRole('heading', { name: 'New custom field', level: 1 })).toBeInTheDocument()
    expect(screen.getByLabelText('Used for')).toHaveValue('identity')
    await userEvent.type(screen.getByLabelText('Name'), 'Slogan')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/identities/edit_default'))
    expect(sent).toEqual({ custom_field: { name: 'Slogan', source: 'identity' } })
  })

  it('never returns to another site', async () => {
    server.use(...loaded(), http.post('*/api/v2/11/custom_fields', () => HttpResponse.json(slogan, { status: 201 })))
    renderAt('/11/custom_fields/new?return_to=https%3A%2F%2Fevil.example')
    expect(await screen.findByRole('link', { name: 'Go back' })).toHaveAttribute('href', '/11/settings/company')
    await userEvent.type(screen.getByLabelText('Name'), 'X')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/settings/company'))
  })
})

describe('management settings', () => {
  it('shows a problem beside its field', async () => {
    server.use(
      ...loaded(),
      http.patch('*/api/v2/administrations/11', () => HttpResponse.json({ error: { name: ["can't be blank"] } }, { status: 422 })),
    )
    renderAt('/11/settings/administration')
    const name = await screen.findByLabelText('Name')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(name).toHaveAccessibleDescription("Name can't be blank"))
  })

  it('saves the administration details', async () => {
    let sent: unknown
    server.use(
      ...loaded(),
      http.patch('*/api/v2/administrations/11', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(harpy)
      }),
    )
    renderAt('/11/settings/administration')
    const name = await screen.findByLabelText('Name')
    await userEvent.clear(name)
    await userEvent.type(name, 'Renamed')
    await userEvent.selectOptions(screen.getByLabelText('Language'), 'nl')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(sent).toEqual({ administration: { name: 'Renamed', language: 'nl', currency: 'EUR', time_zone: 'Europe/Amsterdam' } }),
    )
    expect(await screen.findByRole('status')).toHaveTextContent('Saved')
  })
})
