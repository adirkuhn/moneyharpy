import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { listing, server, signedIn, user } from '../test/server'

const token = { id: '5', name: 'ci', created_at: '2026-11-10T10:00:00Z' }

describe('API tokens', () => {
  it('shows a new secret once and lists the token', async () => {
    server.use(
      signedIn(),
      listing([]),
      http.get('*/api/user/tokens', () => HttpResponse.json([])),
      http.post('*/api/user/tokens', () => HttpResponse.json({ ...token, token: 'mh_secret' }, { status: 201 })),
    )
    renderAt('/user')
    await userEvent.type(await screen.findByLabelText('Token name'), 'ci')
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }))
    expect(await screen.findByText('mh_secret')).toBeInTheDocument()
    expect(screen.getByText('Copy this token now. It will not be shown again.')).toBeInTheDocument()
  })

  it('revokes a token', async () => {
    let revoked = false
    server.use(
      signedIn(),
      listing([]),
      http.get('*/api/user/tokens', () => HttpResponse.json(revoked ? [] : [token])),
      http.delete('*/api/user/tokens/5', () => {
        revoked = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderAt('/user')
    await userEvent.click(await screen.findByRole('button', { name: 'Revoke ci' }))
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Revoke ci' })).toBeNull())
  })
})

describe('instance settings', () => {
  const current = { registration_open: true, google_enabled: false, google_client_id: '', google_client_secret_set: false, lm2_enabled: false, lm2_url: '', lm2_app: '' }

  it('is closed to regular users', async () => {
    server.use(signedIn(), listing([]))
    renderAt('/instance/settings')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })

  it('saves only what the admin filled in', async () => {
    let sent: unknown
    server.use(
      signedIn({ ...user, instance_admin: true }),
      listing([]),
      http.get('*/api/instance/settings', () => HttpResponse.json(current)),
      http.patch('*/api/instance/settings', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json({ ...current, registration_open: false })
      }),
    )
    renderAt('/instance/settings')
    await userEvent.click(await screen.findByLabelText('Allow new users to register'))
    await userEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    await waitFor(() =>
      expect(sent).toEqual({ registration_open: false, google_enabled: false, google_client_id: '', lm2_enabled: false, lm2_url: '', lm2_app: '' }),
    )
    expect(await screen.findByRole('status')).toHaveTextContent('Settings saved')
  })

  it('configures LM2', async () => {
    let sent: unknown
    server.use(
      signedIn({ ...user, instance_admin: true }),
      listing([]),
      http.get('*/api/instance/settings', () => HttpResponse.json(current)),
      http.patch('*/api/instance/settings', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(current)
      }),
    )
    renderAt('/instance/settings')
    await userEvent.click(await screen.findByLabelText('Let users continue with LM2'))
    await userEvent.type(screen.getByLabelText('LM2 address'), 'https://lm.example.com')
    await userEvent.type(screen.getByLabelText('App name in LM2'), 'moneyharpy')
    await userEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    await waitFor(() =>
      expect(sent).toMatchObject({ lm2_enabled: true, lm2_url: 'https://lm.example.com', lm2_app: 'moneyharpy' }),
    )
  })

  it('keeps a stored secret unless a new one is typed', async () => {
    server.use(
      signedIn({ ...user, instance_admin: true }),
      listing([]),
      http.get('*/api/instance/settings', () =>
        HttpResponse.json({ ...current, google_enabled: true, google_client_id: 'id', google_client_secret_set: true }),
      ),
    )
    renderAt('/instance/settings')
    expect(await screen.findByLabelText('Google client secret')).toHaveAttribute('placeholder', 'Stored. Type to replace it.')
  })
})
