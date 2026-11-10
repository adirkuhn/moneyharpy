import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn } from '../test/server'

const createPath = '/onboarding/company/create_administration?origin=onboarding'
const manualPath = '/onboarding/company/manually?country=NL'

const options = (over = {}) =>
  http.get('*/api/onboarding/options', () =>
    HttpResponse.json({
      countries: ['BE', 'DE', 'NL'],
      currencies: ['EUR', 'GBP', 'JPY', 'USD'],
      popular_currencies: ['EUR', 'USD', 'GBP'],
      company_search_countries: [],
      address_lookup_countries: [],
      ...over,
    }),
  )

function creating(onBody: (body: unknown) => void = () => {}) {
  return http.post('*/api/v2/administrations', async ({ request }) => {
    onBody(await request.json())
    return HttpResponse.json(administration({ id: '33', name: 'New Co' }), { status: 201 })
  })
}

async function choose(label: string, option: string) {
  await userEvent.click(screen.getByRole('combobox', { name: label }))
  await userEvent.click(await screen.findByRole('option', { name: option }))
}

describe('starting to add an administration', () => {
  it('welcomes the user in a screen of its own', async () => {
    server.use(signedIn(), options())
    renderAt(createPath)
    expect(await screen.findByRole('heading', { name: 'Welcome Ann' })).toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: 'Main' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/administrations')
  })

  it('goes to the manual form while no register is configured', async () => {
    server.use(signedIn(), options())
    renderAt(createPath)
    expect(await screen.findByRole('link', { name: 'Next' })).toHaveAttribute('href', manualPath)
    expect(screen.queryByRole('textbox', { name: 'What is the name of the company?' })).toBeNull()
  })

  describe('with the register configured', () => {
    const results = [
      { name: 'Harpy B.V.', chamber_of_commerce: '12345678', address1: 'Oosterwal', zipcode: '', city: 'Lochem' },
    ]
    const searching = (rows: unknown[] = results) =>
      http.get('*/api/onboarding/company_search', () => HttpResponse.json(rows))
    const configured = options({ company_search_countries: ['NL'] })

    it('finds a company, then lets the user complete it', async () => {
      server.use(signedIn(), configured, searching())
      renderAt(createPath)

      const next = await screen.findByRole('button', { name: 'Next' })
      expect(next).toBeDisabled()

      await userEvent.type(screen.getByRole('textbox', { name: 'What is the name of the company?' }), 'harpy')
      await userEvent.click(await screen.findByRole('radio', { name: /Harpy B\.V\./ }))
      expect(next).toBeEnabled()
      await userEvent.click(next)

      await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/onboarding/company/manually'))
      expect(await screen.findByLabelText('Company name')).toHaveValue('Harpy B.V.')
      expect(screen.getByLabelText('City')).toHaveValue('Lochem')
    })

    it('carries the register number into the new administration', async () => {
      let sent: { administration?: Record<string, string> } = {}
      server.use(signedIn(), configured, searching(), creating((b) => (sent = b as typeof sent)))
      renderAt(createPath)
      await userEvent.type(await screen.findByRole('textbox', { name: 'What is the name of the company?' }), 'harpy')
      await userEvent.click(await screen.findByRole('radio', { name: /Harpy B\.V\./ }))
      await userEvent.click(screen.getByRole('button', { name: 'Next' }))
      await userEvent.click(await screen.findByRole('button', { name: 'Next' }))
      await waitFor(() => expect(sent.administration?.chamber_of_commerce).toBe('12345678'))
    })

    it('says when nothing matches', async () => {
      server.use(signedIn(), configured, searching([]))
      renderAt(createPath)
      await userEvent.type(await screen.findByRole('textbox', { name: 'What is the name of the company?' }), 'zzz')
      expect(await screen.findByText('No company found. Check the name or enter the details yourself.')).toBeInTheDocument()
    })

    it('offers the manual form when the register fails', async () => {
      server.use(signedIn(), configured, http.get('*/api/onboarding/company_search', () => HttpResponse.json({ error: 'x' }, { status: 500 })))
      renderAt(createPath)
      await userEvent.type(await screen.findByRole('textbox', { name: 'What is the name of the company?' }), 'harpy')
      expect(await screen.findByRole('alert')).toHaveTextContent('The company register could not be reached.')
    })

    it('links to the manual form for companies outside the register', async () => {
      server.use(signedIn(), configured)
      renderAt(createPath)
      expect(await screen.findByRole('link', { name: 'The company is not registered yet, or is not Dutch' })).toHaveAttribute('href', manualPath)
    })
  })
})

describe('entering a company by hand', () => {
  it('sends everything needed to create the administration', async () => {
    let sent: unknown
    server.use(signedIn(), options(), creating((b) => (sent = b)))
    renderAt(manualPath)

    await userEvent.type(await screen.findByLabelText('Company name'), 'New Co')
    await userEvent.type(screen.getByLabelText('Address'), 'Dam 1')
    await userEvent.type(screen.getByLabelText('Zipcode'), '1012JS')
    await userEvent.type(screen.getByLabelText('City'), 'Amsterdam')
    await choose('Currency', 'USD - US Dollar')
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))

    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/33/onboarding/first_action/new'))
    expect(sent).toEqual({
      administration: {
        name: 'New Co',
        country: 'NL',
        currency: 'USD',
        language: 'en',
        time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        address1: 'Dam 1',
        address2: '',
        zipcode: '1012JS',
        city: 'Amsterdam',
        chamber_of_commerce: '',
      },
    })
  })

  it('offers popular currencies first', async () => {
    server.use(signedIn(), options())
    renderAt(manualPath)
    await userEvent.click(await screen.findByRole('combobox', { name: 'Currency' }))
    const names = within(await screen.findByRole('listbox')).getAllByRole('option').map((o) => o.textContent)
    expect(names.slice(0, 3)).toEqual(['EUR - Euro', 'USD - US Dollar', 'GBP - British Pound'])
  })

  it('searches the country list', async () => {
    server.use(signedIn(), options())
    renderAt(manualPath)
    await userEvent.click(await screen.findByRole('combobox', { name: 'Country' }))
    await userEvent.type(await screen.findByRole('searchbox', { name: 'Find country' }), 'germ')
    expect(within(screen.getByRole('listbox')).getAllByRole('option').map((o) => o.textContent)).toEqual(['Germany'])
  })

  it('asks for a register number only outside the Netherlands and Belgium', async () => {
    server.use(signedIn(), options())
    renderAt(manualPath)
    await screen.findByLabelText('Company name')
    expect(screen.queryByLabelText('Chamber of Commerce number')).toBeNull()
    await choose('Country', 'Germany')
    expect(screen.getByLabelText('Chamber of Commerce number')).toBeInTheDocument()
    await choose('Country', 'Belgium')
    expect(screen.queryByLabelText('Chamber of Commerce number')).toBeNull()
  })

  it('shows and removes the extra address line', async () => {
    server.use(signedIn(), options())
    renderAt(manualPath)
    await screen.findByLabelText('Company name')
    expect(screen.queryByLabelText('Additional address details')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Add address details' }))
    expect(screen.getByLabelText('Additional address details')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Remove address details' }))
    expect(screen.queryByLabelText('Additional address details')).toBeNull()
  })

  it('shows which field is wrong', async () => {
    server.use(
      signedIn(),
      options(),
      http.post('*/api/v2/administrations', () => HttpResponse.json({ error: { name: ["can't be blank"] } }, { status: 422 })),
    )
    renderAt(manualPath)
    await userEvent.type(await screen.findByLabelText('Company name'), 'x')
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByText("Company name can't be blank")).toBeInTheDocument()
  })

  describe('with address lookup on', () => {
    const lookupOn = options({ address_lookup_countries: ['NL'] })

    it('completes the address from zipcode and house number', async () => {
      server.use(
        signedIn(),
        lookupOn,
        http.get('*/api/onboarding/address_lookup', ({ request }) => {
          const q = new URL(request.url).searchParams
          expect([q.get('zipcode'), q.get('house_number')]).toEqual(['1012RJ', '147'])
          return HttpResponse.json({ address1: 'Voorburgwal 147', zipcode: '1012RJ', city: 'Amsterdam' })
        }),
      )
      renderAt(manualPath)
      await userEvent.type(await screen.findByLabelText('Zipcode'), '1012RJ')
      await userEvent.type(screen.getByLabelText('House number'), '147')
      await userEvent.tab()
      await waitFor(() => expect(screen.getByLabelText('Address')).toHaveValue('Voorburgwal 147'))
      expect(screen.getByLabelText('City')).toHaveValue('Amsterdam')
    })

    it('warns about an unknown address without blocking', async () => {
      server.use(signedIn(), lookupOn, http.get('*/api/onboarding/address_lookup', () => HttpResponse.json({ error: 'x' }, { status: 404 })))
      renderAt(manualPath)
      await userEvent.type(await screen.findByLabelText('Zipcode'), '9999ZZ')
      await userEvent.type(screen.getByLabelText('House number'), '1')
      await userEvent.tab()
      expect(await screen.findByText('Combination of zipcode and house number is unknown')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Next' })).toBeEnabled()
    })

    it('hides the house number for other countries', async () => {
      server.use(signedIn(), lookupOn)
      renderAt(manualPath)
      await screen.findByLabelText('House number')
      await choose('Country', 'Germany')
      expect(screen.queryByLabelText('House number')).toBeNull()
    })
  })
})

describe('choosing what to do first', () => {
  const path = '/33/onboarding/first_action/new?origin=onboarding'
  const harpy = administration({ id: '33', name: 'New Co' })
  const loaded = [signedIn(), listing([harpy]), http.get('*/api/v2/administrations/33', () => HttpResponse.json(harpy))]

  it('offers the four starting points', async () => {
    server.use(...loaded)
    renderAt(path)
    expect(await screen.findByRole('heading', { name: 'What do you want to start with?' })).toBeInTheDocument()
    expect(screen.getAllByRole('radio')).toHaveLength(4)
  })

  it('requires a choice, then opens the administration', async () => {
    server.use(...loaded)
    renderAt(path)
    const start = await screen.findByRole('button', { name: 'Get started' })
    expect(start).toBeDisabled()
    await userEvent.click(screen.getByRole('radio', { name: /open my administration/ }))
    await userEvent.click(start)
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(/^\/33$/))
  })
})

describe('the wizard language switcher', () => {
  it('translates the wizard', async () => {
    server.use(signedIn(), options())
    renderAt(createPath)
    await userEvent.selectOptions(await screen.findByLabelText('Language'), 'nl')
    expect(await screen.findByRole('heading', { name: 'Welkom Ann' })).toBeInTheDocument()
  })
})
