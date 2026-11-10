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
    expect(await screen.findByRole('heading', { name: 'Sign in to MoneyHarpy' })).toBeInTheDocument()
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
    await userEvent.type(await screen.findByLabelText('Email'), 'ann@example.com')
    await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))
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
    await userEvent.type(await screen.findByLabelText('Email'), 'a@b.nl')
    await userEvent.type(screen.getByLabelText('Password'), 'nope')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))
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
    await screen.findByRole('heading', { name: 'Sign in to MoneyHarpy' })
    await waitFor(() => expect(screen.queryByRole('link', { name: 'Create an account' })).toBeNull())
    expect(screen.queryByRole('link', { name: 'Continue with Google' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Continue with LM2' })).toBeNull()
  })

  it('shows why a Google sign-in failed', async () => {
    server.use(signedOut(), providers(true))
    renderAt('/login?error=registration_closed')
    expect(await screen.findByRole('alert')).toHaveTextContent('Registration is closed')
  })
})

describe('registering', () => {
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
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.nl')
    await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }))
    expect(await screen.findByText('Email has already been taken')).toBeInTheDocument()
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
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.nl')
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
    expect(await screen.findByRole('heading', { name: 'Sign in to MoneyHarpy' })).toBeInTheDocument()
  })
})
