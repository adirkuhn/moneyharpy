import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn, signedOut, user } from '../test/server'
import type { User } from './queries'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })
const stamp = '2026-11-10T09:00:00Z'
const member = (over: Partial<User> = {}): User => ({
  id: '1', name: 'Ann de Vries', email: 'ann@example.com', email_validated: false, language: 'en', time_zone: 'Europe/Amsterdam',
  user_type: 'owner', permissions: ['sales_invoices', 'documents', 'estimates', 'bank', 'settings', 'ownership', 'time_entries'],
  active: true, created_at: stamp, updated_at: stamp, ...over,
})
const clerk = member({ id: '2', name: 'Clerk', email: 'clerk@example.com', user_type: 'employee', permissions: ['bank'] })

const loaded = (a = harpy, me = user) => [
  signedIn(me),
  listing([a]),
  http.get('*/api/v2/administrations/11', () => HttpResponse.json(a)),
  http.get('*/api/v2/11/users', () => HttpResponse.json([member(), clerk])),
  http.get('*/api/v2/11/user_invitations', () =>
    HttpResponse.json([{ id: '91', email: 'new@example.com', user_type: 'accountant', permissions: ['bank'], created_at: stamp, expires_at: '2026-11-24T09:00:00Z', expired: false }]),
  ),
  http.get('*/api/onboarding/options', () => HttpResponse.json({ countries: ['NL', 'BE'], currencies: ['EUR'], popular_currencies: ['EUR'], company_search_countries: [], address_lookup_countries: [] })),
]

describe('management settings', () => {
  it('shows every part with users and open invitations', async () => {
    server.use(...loaded())
    renderAt('/11/settings/administration')
    expect(await screen.findByRole('heading', { name: 'Management', level: 1 })).toBeInTheDocument()
    for (const name of ['Administration details', 'Users', 'Security', 'Privacy and access', 'Leave or delete']) {
      expect(screen.getByRole('region', { name })).toBeInTheDocument()
    }
    const users = await screen.findByRole('table', { name: 'Users' })
    expect(await within(users).findByRole('link', { name: 'Clerk' })).toHaveAttribute('href', '/11/users/2')
    expect(within(users).getByText('Employee')).toBeInTheDocument()
    expect(await screen.findByRole('table', { name: 'Open invitations' })).toHaveTextContent('new@example.com')
    expect(screen.getByRole('link', { name: 'Delete administration' })).toHaveAttribute('href', '/11/delete')
    expect(screen.getByText('You are the only owner. Make someone else an owner first.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Leave' })).toBeDisabled()
  })

  it('saves the details, country included', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/administrations/11', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(harpy)
    }))
    renderAt('/11/settings/administration')
    const name = await screen.findByLabelText('Administration name')
    await userEvent.clear(name)
    await userEvent.type(name, 'Renamed')
    await userEvent.selectOptions(screen.getByLabelText('Language'), 'nl')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ administration: { name: 'Renamed', country: 'NL', language: 'nl', currency: 'EUR', time_zone: 'Europe/Amsterdam' } }))
  })

  it('shows a problem beside its field', async () => {
    server.use(...loaded(), http.patch('*/api/v2/administrations/11', () => HttpResponse.json({ error: { name: ["can't be blank"] } }, { status: 422 })))
    renderAt('/11/settings/administration')
    const name = await screen.findByLabelText('Administration name')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(name).toHaveAccessibleDescription("Administration name can't be blank"))
  })

  it('asks the owner to set up two-step verification before requiring it', async () => {
    server.use(...loaded())
    renderAt('/11/settings/administration')
    await userEvent.click(await screen.findByRole('switch', { name: 'Require two-step verification' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Set up two-step verification for your own account first.')
    expect(screen.getByRole('link', { name: 'Set up two-step verification' })).toHaveAttribute('href', '/user#two-step')
  })

  it('requires two-step verification after confirming', async () => {
    let required = false
    server.use(...loaded(harpy, { ...user, two_factor_enabled: true }), http.put('*/api/v2/11/require_two_factor', () => {
      required = true
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/settings/administration')
    await userEvent.click(await screen.findByRole('switch', { name: 'Require two-step verification' }))
    expect(required).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: 'Require two-step verification' }))
    await waitFor(() => expect(required).toBe(true))
  })

  it('shows an employee only what they may use', async () => {
    const limited = administration({ id: '11', name: 'Harpy B.V.', user_type: 'employee', permissions: ['bank'] })
    server.use(...loaded(limited))
    renderAt('/11/settings/administration')
    expect(await screen.findByRole('heading', { name: 'Management', level: 1 })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Users' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Delete administration' })).not.toBeInTheDocument()
    const nav = screen.getByRole('navigation', { name: 'Settings' })
    expect(within(nav).queryByRole('link', { name: 'Bookkeeping' })).not.toBeInTheDocument()
    expect(screen.getByLabelText('Administration name')).toBeDisabled()
    expect(screen.getByRole('link', { name: 'Bank' })).toBeInTheDocument()
  })

  it('tells someone without access to a section', async () => {
    const limited = administration({ id: '11', name: 'Harpy B.V.', user_type: 'employee', permissions: ['bank'] })
    server.use(...loaded(limited))
    renderAt('/11/settings/bookkeeping')
    expect(await screen.findByRole('heading', { name: 'No access' })).toBeInTheDocument()
  })
})

describe('users and invitations', () => {
  it('invites with the type defaults and shows the link when email is off', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/11/user_invitations', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ id: '92', email: 'acc@example.com', user_type: 'accountant', permissions: [], created_at: stamp, expires_at: stamp, expired: false, accept_url: 'https://example.com/invitations/abc', email_sent: false }, { status: 201 })
    }))
    renderAt('/11/user_invitations/new?type=accountant')
    await userEvent.type(await screen.findByLabelText('Email address'), 'acc@example.com')
    expect(screen.getByRole('checkbox', { name: 'Settings' })).toBeChecked()
    await userEvent.click(screen.getByRole('checkbox', { name: 'Time tracking' }))
    await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))
    await waitFor(() =>
      expect(sent).toEqual({ user_invitation: { email: 'acc@example.com', user_type: 'accountant', permissions: ['sales_invoices', 'documents', 'estimates', 'bank', 'settings'] } }),
    )
    expect(await screen.findByRole('status')).toHaveTextContent('https://example.com/invitations/abc')
  })

  it('changes a user and keeps ownership with the owner type', async () => {
    let sent: unknown
    server.use(...loaded(), http.patch('*/api/v2/11/users/2', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json(clerk)
    }))
    renderAt('/11/users/2')
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Settings' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).toEqual({ user: { user_type: 'employee', permissions: ['bank', 'settings'], active: true } }))
    await userEvent.selectOptions(screen.getByLabelText('Access rights'), 'owner')
  })

  it('accepts an invitation when signed in', async () => {
    let accepted = false
    server.use(
      ...loaded(),
      http.get('*/api/invitations/:token', () => HttpResponse.json({ administration_name: 'Harpy B.V.', inviter_name: 'Ann', email: 'me@example.com', user_type: 'employee', open: true })),
      http.post('*/api/invitations/:token/accept', () => {
        accepted = true
        return HttpResponse.json(harpy)
      }),
    )
    renderAt('/invitations/abcdefghijklmnopqrstuvwxyz')
    expect(await screen.findByText('Ann invited you to Harpy B.V. as Employee.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Accept invitation' }))
    await waitFor(() => expect(accepted).toBe(true))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11'))
  })

  it('offers signing in or registering to a visitor', async () => {
    server.use(
      signedOut(),
      http.get('*/api/invitations/:token', () => HttpResponse.json({ administration_name: 'Harpy B.V.', inviter_name: '', email: 'me@example.com', user_type: 'accountant', open: true })),
    )
    renderAt('/invitations/abcdefghijklmnopqrstuvwxyz')
    expect(await screen.findByRole('link', { name: 'Create an account' })).toHaveAttribute('href', '/register?invitation=abcdefghijklmnopqrstuvwxyz&email=me%40example.com')
  })
})

describe('leave, delete and support access', () => {
  it('deletes only after typing the name', async () => {
    let sent: unknown
    server.use(...loaded(), http.post('*/api/v2/administrations/11/delete', async ({ request }) => {
      sent = await request.json()
      return new HttpResponse(null, { status: 204 })
    }))
    renderAt('/11/delete')
    const button = await screen.findByRole('button', { name: 'Delete permanently' })
    expect(button).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Administration name'), 'Harpy B.V.')
    await userEvent.click(button)
    await waitFor(() => expect(sent).toEqual({ name: 'Harpy B.V.' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/administrations'))
  })

  it('grants support access for a number of days', async () => {
    let until = ''
    server.use(...loaded(), http.put('*/api/v2/11/support_access', async ({ request }) => {
      until = ((await request.json()) as { until: string }).until
      return HttpResponse.json({ until })
    }))
    renderAt('/11/support_access')
    expect(await screen.findByText('Support access is off.')).toBeInTheDocument()
    await userEvent.selectOptions(screen.getByLabelText('For'), '30')
    await userEvent.click(screen.getByRole('button', { name: 'Give access' }))
    await waitFor(() => expect(Date.parse(until) - Date.now()).toBeGreaterThan(29 * 24 * 3600 * 1000))
  })

  it('shows the privacy statements', async () => {
    server.use(...loaded())
    renderAt('/11/privacy/data_analysis')
    expect(await screen.findByText(/collects no usage data/)).toBeInTheDocument()
  })
})

describe('two-step verification', () => {
  it('asks for a code after the password and then signs in', async () => {
    let code = ''
    server.use(
      signedOut(),
      listing([harpy]),
      http.get('*/api/auth/providers', () => HttpResponse.json({ google: false, registration_open: true, lm2: false, password_reset: false })),
      http.post('*/api/auth/login', () => HttpResponse.json({ two_factor_required: true }, { status: 202 })),
      http.post('*/api/auth/two_factor', async ({ request }) => {
        code = ((await request.json()) as { code: string }).code
        return HttpResponse.json({ ...user, two_factor_enabled: true })
      }),
    )
    renderAt('/login')
    await userEvent.type(await screen.findByLabelText('Email address'), 'ann@example.com')
    await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
    await userEvent.click(screen.getByRole('button', { name: 'Login to MoneyHarpy' }))
    await userEvent.type(await screen.findByLabelText('Code'), '123456')
    await userEvent.click(screen.getByRole('button', { name: 'Verify' }))
    await waitFor(() => expect(code).toBe('123456'))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/administrations'))
  })

  it('sets up two-step verification and shows the recovery codes', async () => {
    server.use(
      signedIn(),
      listing([]),
      http.get('*/api/user/tokens', () => HttpResponse.json([])),
      http.get('*/api/user/two_factor', () => HttpResponse.json({ enabled: false, required: true, recovery_codes_left: 0 })),
      http.post('*/api/user/two_factor', () => HttpResponse.json({ secret: 'ABCDEF', otpauth_uri: 'otpauth://totp/x', qr_svg: '<svg/>' }, { status: 201 })),
      http.post('*/api/user/two_factor/confirm', () => HttpResponse.json({ recovery_codes: ['aaaa-bbbb-cccc', 'dddd-eeee-ffff'] })),
    )
    renderAt('/user')
    expect(await screen.findByText('An administration you work in requires two-step verification.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Set up two-step verification' }))
    expect(await screen.findByRole('img', { name: 'QR code for your authenticator app' })).toBeInTheDocument()
    expect(screen.getByText('ABCDEF')).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Code'), '123456')
    await userEvent.click(screen.getByRole('button', { name: 'Turn on' }))
    expect(await screen.findByText('aaaa-bbbb-cccc')).toBeInTheDocument()
  })

  it('warns in an administration that requires it', async () => {
    server.use(...loaded(administration({ id: '11', name: 'Harpy B.V.', require_two_factor: true })))
    renderAt('/11/settings/administration')
    expect(await screen.findByText('Harpy B.V. requires two-step verification. Set it up to keep working in it.')).toBeInTheDocument()
  })
})
