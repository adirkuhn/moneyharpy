import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { listing, providers, server, signedOut, user } from '../test/server'

describe('signing in', () => {
  it('sends visitors to the sign-in page', async () => {
    server.use(signedOut(), providers())
    renderAt('/administrations')
    expect(await screen.findByRole('heading', { name: 'Login' })).toBeInTheDocument()
    expect(screen.getByTestId('path')).toHaveTextContent('/login')
  })

  it('opens the requested page after signing in', async () => {
    let authed = false
    server.use(
      http.get('*/api/auth/me', () =>
        authed ? HttpResponse.json(user) : HttpResponse.json({ error: 'x' }, { status: 401 }),
      ),
      http.post('*/api/auth/login', () => {
        authed = true
        return HttpResponse.json(user)
      }),
      providers(),
      listing([]),
    )
    renderAt('/user')
    await userEvent.type(await screen.findByLabelText('Email address'), 'ann@example.com')
    await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
    await userEvent.click(screen.getByRole('button', { name: 'Login to MoneyHarpy' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/user'))
  })

  it('explains a rejected password', async () => {
    server.use(
      signedOut(),
      providers(),
      http.post('*/api/auth/login', () =>
        HttpResponse.json({ error: 'Invalid email or password' }, { status: 401 }),
      ),
    )
    renderAt('/login')
    await userEvent.type(await screen.findByLabelText('Email address'), 'a@b.nl')
    await userEvent.type(screen.getByLabelText('Password'), 'nope')
    await userEvent.click(screen.getByRole('button', { name: 'Login to MoneyHarpy' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid email or password')
  })

  it('offers Google only when it is enabled', async () => {
    server.use(signedOut(), providers(true))
    renderAt('/login')
    const link = await screen.findByRole('link', { name: 'Continue with Google' })
    expect(link).toHaveAttribute('href', '/api/auth/google/start')
  })

  it('offers LM2 only when it is enabled', async () => {
    server.use(signedOut(), providers(false, true, true))
    renderAt('/login')
    const link = await screen.findByRole('link', { name: 'Continue with LM2' })
    expect(link).toHaveAttribute('href', '/api/auth/lm2/start')
  })

  it('explains a failed LM2 sign-in', async () => {
    server.use(signedOut(), providers(false, true, true))
    renderAt('/login?error=lm2')
    expect(await screen.findByRole('alert')).toHaveTextContent('LM2 sign-in did not complete')
  })

  it('hides Google and registration when they are off', async () => {
    server.use(signedOut(), providers(false, false))
    renderAt('/login')
    await screen.findByRole('heading', { name: 'Login' })
    await waitFor(() => expect(screen.queryByRole('link', { name: 'No account?' })).toBeNull())
    expect(screen.queryByRole('link', { name: 'Continue with Google' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Continue with LM2' })).toBeNull()
  })

  it('shows why a Google sign-in failed', async () => {
    server.use(signedOut(), providers(true))
    renderAt('/login?error=registration_closed')
    expect(await screen.findByRole('alert')).toHaveTextContent('Registration is closed')
  })
})

describe('the sign-in screen', () => {
  it('offers password recovery only when mail is configured', async () => {
    server.use(signedOut(), providers(false, true, false, true))
    renderAt('/login')
    expect(await screen.findByRole('link', { name: 'Forgot your password?' })).toHaveAttribute('href', '/passwords/new')
  })

  it('hides password recovery without mail', async () => {
    server.use(signedOut(), providers())
    renderAt('/login')
    await screen.findByRole('link', { name: 'No account?' })
    expect(screen.queryByRole('link', { name: 'Forgot your password?' })).toBeNull()
  })

  it('separates other sign-in methods only when there are some', async () => {
    server.use(signedOut(), providers())
    renderAt('/login')
    await screen.findByRole('link', { name: 'No account?' })
    expect(screen.queryByText('or')).toBeNull()
  })

  it('shows the divider above other methods', async () => {
    server.use(signedOut(), providers(true))
    renderAt('/login')
    expect(await screen.findByText('or')).toBeInTheDocument()
  })

  it('switches language', async () => {
    server.use(signedOut(), providers())
    renderAt('/login')
    await userEvent.selectOptions(await screen.findByLabelText('Language'), 'nl')
    expect(await screen.findByRole('heading', { name: 'Inloggen' })).toBeInTheDocument()
  })
})

describe('recovering a password', () => {
  it('requests a link and does not reveal whether the account exists', async () => {
    let sent: unknown
    server.use(
      signedOut(),
      providers(false, true, false, true),
      http.post('*/api/auth/password_reset/request', async ({ request }) => {
        sent = await request.json()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/passwords/new')
    await userEvent.type(await screen.findByLabelText('Email address'), 'ann@example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Send reset link' }))
    expect(await screen.findByRole('status')).toHaveTextContent('If this address has an account, we sent a link to it.')
    expect(sent).toEqual({ email: 'ann@example.com', language: 'en' })
  })

  it('sets the new password and returns to sign in', async () => {
    let sent: unknown
    server.use(
      signedOut(),
      providers(),
      http.post('*/api/auth/password_reset/confirm', async ({ request }) => {
        sent = await request.json()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/passwords/edit?token=abc')
    await userEvent.type(await screen.findByLabelText('New password'), 'a brand new password')
    await userEvent.click(screen.getByRole('button', { name: 'Save new password' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/login'))
    expect(sent).toEqual({ token: 'abc', password: 'a brand new password' })
    expect(await screen.findByRole('status')).toHaveTextContent('Password changed. Log in with your new password.')
  })

  it('explains an expired link and offers a new one', async () => {
    server.use(
      signedOut(),
      providers(),
      http.post('*/api/auth/password_reset/confirm', () =>
        HttpResponse.json({ error: { token: ['is invalid or has expired'] } }, { status: 422 }),
      ),
    )
    renderAt('/passwords/edit?token=old')
    await userEvent.type(await screen.findByLabelText('New password'), 'a brand new password')
    await userEvent.click(screen.getByRole('button', { name: 'Save new password' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('This link is invalid or has expired')
    expect(screen.getByRole('link', { name: 'Request a new link' })).toHaveAttribute('href', '/passwords/new')
  })

  it('handles an address without a token', async () => {
    server.use(signedOut(), providers())
    renderAt('/passwords/edit')
    expect(await screen.findByRole('alert')).toHaveTextContent('This link is invalid or has expired')
    expect(screen.queryByLabelText('New password')).toBeNull()
  })
})

describe('registering', () => {
  it('links back to sign in', async () => {
    server.use(signedOut(), providers())
    renderAt('/register')
    expect(await screen.findByRole('link', { name: 'Already have an account?' })).toHaveAttribute('href', '/login')
  })

  it('shows field errors from the server', async () => {
    server.use(
      signedOut(),
      providers(),
      http.post('*/api/auth/register', () =>
        HttpResponse.json({ error: { email: ['has already been taken'] } }, { status: 422 }),
      ),
    )
    renderAt('/register')
    await userEvent.type(await screen.findByLabelText('Name'), 'Ann')
    await userEvent.type(screen.getByLabelText('Email address'), 'a@b.nl')
    await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }))
    expect(await screen.findByText('Email address has already been taken')).toBeInTheDocument()
  })

  it('lands on administrations', async () => {
    let authed = false
    server.use(
      http.get('*/api/auth/me', () =>
        authed ? HttpResponse.json(user) : HttpResponse.json({ error: 'x' }, { status: 401 }),
      ),
      http.post('*/api/auth/register', () => {
        authed = true
        return HttpResponse.json(user, { status: 201 })
      }),
      providers(),
      listing([]),
    )
    renderAt('/register')
    await userEvent.type(await screen.findByLabelText('Name'), 'Ann')
    await userEvent.type(screen.getByLabelText('Email address'), 'a@b.nl')
    await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/administrations'))
  })
})

describe('signing out', () => {
  it('returns to the sign-in page', async () => {
    let authed = true
    server.use(
      http.get('*/api/auth/me', () =>
        authed ? HttpResponse.json(user) : HttpResponse.json({ error: 'x' }, { status: 401 }),
      ),
      http.post('*/api/auth/logout', () => {
        authed = false
        return new HttpResponse(null, { status: 204 })
      }),
      providers(),
      listing([]),
    )
    renderAt('/administrations')
    await userEvent.click(await screen.findByRole('button', { name: 'Ann de Vries' }))
    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }))
    expect(await screen.findByRole('heading', { name: 'Login' })).toBeInTheDocument()
  })
})
