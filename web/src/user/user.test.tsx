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
  const current = { registration_open: true, google_enabled: false, google_client_id: '', google_client_secret_set: false, lm2_enabled: false, lm2_url: '', lm2_app: '', kvk_enabled: false, kvk_api_key_set: false, address_lookup_enabled: false, smtp_enabled: false, smtp_host: '', smtp_port: 0, smtp_username: '', smtp_password_set: false, smtp_from: '', smtp_security: '', storage_enabled: false, storage_endpoint: '', storage_region: '', storage_bucket: '', storage_access_key: '', storage_secret_key_set: false, storage_path_style: false }

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
      expect(sent).toEqual({ registration_open: false, google_enabled: false, google_client_id: '', lm2_enabled: false, lm2_url: '', lm2_app: '', kvk_enabled: false, address_lookup_enabled: false, smtp_enabled: false, smtp_host: '', smtp_port: 587, smtp_username: '', smtp_from: '', smtp_security: 'starttls', storage_enabled: false, storage_endpoint: '', storage_region: '', storage_bucket: '', storage_access_key: '', storage_path_style: false }),
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

  it('adds company search and address lookup later', async () => {
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
    await userEvent.click(await screen.findByLabelText('Search the Dutch company register'))
    await userEvent.type(screen.getByLabelText('KVK API key'), 'kvk-key')
    await userEvent.click(screen.getByLabelText('Complete Dutch addresses from zipcode and house number'))
    await userEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    await waitFor(() =>
      expect(sent).toMatchObject({ kvk_enabled: true, kvk_api_key: 'kvk-key', address_lookup_enabled: true }),
    )
  })

  it('configures the mail server', async () => {
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
    await userEvent.click(await screen.findByLabelText('Send email through this mail server'))
    await userEvent.type(screen.getByLabelText('SMTP host'), 'mail.example.com')
    await userEvent.clear(screen.getByLabelText('SMTP port'))
    await userEvent.type(screen.getByLabelText('SMTP port'), '465')
    await userEvent.type(screen.getByLabelText('SMTP username'), 'books')
    await userEvent.type(screen.getByLabelText('SMTP password'), 'pw')
    await userEvent.type(screen.getByLabelText('Send email from'), 'books@example.com')
    await userEvent.selectOptions(screen.getByLabelText('Encryption'), 'tls')
    await userEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    await waitFor(() =>
      expect(sent).toMatchObject({
        smtp_enabled: true, smtp_host: 'mail.example.com', smtp_port: 465, smtp_username: 'books',
        smtp_password: 'pw', smtp_from: 'books@example.com', smtp_security: 'tls',
      }),
    )
  })

  it('configures file storage and checks it', async () => {
    let sent: unknown
    const saved = { ...current, storage_enabled: true, storage_endpoint: 'http://storage:8333', storage_bucket: 'files', storage_access_key: 'k', storage_secret_key_set: true }
    server.use(
      signedIn({ ...user, instance_admin: true }),
      listing([]),
      http.get('*/api/instance/settings', () => HttpResponse.json(saved)),
      http.patch('*/api/instance/settings', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(saved)
      }),
      http.post('*/api/instance/settings/test_storage', () => new HttpResponse(null, { status: 204 })),
    )
    renderAt('/instance/settings')
    const endpoint = await screen.findByLabelText('Endpoint')
    await userEvent.clear(endpoint)
    await userEvent.type(endpoint, 'https://s3.example.com')
    await userEvent.type(screen.getByLabelText('Secret key'), 'secret')
    await userEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    await waitFor(() => expect(sent).toMatchObject({ storage_enabled: true, storage_endpoint: 'https://s3.example.com', storage_bucket: 'files', storage_secret_key: 'secret', storage_path_style: false }))
    await userEvent.click(screen.getByRole('button', { name: 'Test storage' }))
    expect(await screen.findByText('The storage works')).toBeInTheDocument()
  })

  describe('sending a test email', () => {
    const saved = { ...current, smtp_enabled: true, smtp_host: 'h', smtp_port: 587, smtp_from: 'a@b.nl', smtp_security: 'starttls' }

    it('is only offered for saved, enabled mail settings', async () => {
      server.use(signedIn({ ...user, instance_admin: true }), listing([]), http.get('*/api/instance/settings', () => HttpResponse.json(current)))
      renderAt('/instance/settings')
      await screen.findByLabelText('SMTP host')
      expect(screen.getByRole('button', { name: 'Send test email' })).toBeDisabled()
    })

    it('confirms delivery to the administrator', async () => {
      server.use(
        signedIn({ ...user, instance_admin: true }),
        listing([]),
        http.get('*/api/instance/settings', () => HttpResponse.json(saved)),
        http.post('*/api/instance/settings/test_email', () => new HttpResponse(null, { status: 204 })),
      )
      renderAt('/instance/settings')
      await userEvent.click(await screen.findByRole('button', { name: 'Send test email' }))
      expect(await screen.findByText(`Test email sent to ${user.email}`)).toBeInTheDocument()
    })

    it('shows why the mail server refused', async () => {
      server.use(
        signedIn({ ...user, instance_admin: true }),
        listing([]),
        http.get('*/api/instance/settings', () => HttpResponse.json(saved)),
        http.post('*/api/instance/settings/test_email', () => HttpResponse.json({ error: '535 authentication failed' }, { status: 502 })),
      )
      renderAt('/instance/settings')
      await userEvent.click(await screen.findByRole('button', { name: 'Send test email' }))
      expect(await screen.findByRole('alert')).toHaveTextContent('535 authentication failed')
    })
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
