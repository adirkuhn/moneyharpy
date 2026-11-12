import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { server } from '../test/server'
import { loaded, otherStyle } from './testkit'

describe('document style name and sender address', () => {
  it('adds a style, then opens its editor', async () => {
    let sent: { document_style?: Record<string, unknown> } = {}
    server.use(...loaded(), http.post('*/api/v2/11/document_styles', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(otherStyle, { status: 201 })
    }))
    renderAt('/11/document_styles/new')
    expect(await screen.findByRole('heading', { name: 'New document style', level: 1 })).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Name'), 'The Best')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/document_styles/702/edit/document'))
    expect(sent.document_style).toEqual({ name: 'The Best', identity_id: '500', default: false })
  })

  it('changes the name and sender address', async () => {
    let sent: { document_style?: Record<string, unknown> } = {}
    server.use(...loaded(), http.patch('*/api/v2/11/document_styles/702', async ({ request }) => {
      sent = (await request.json()) as typeof sent
      return HttpResponse.json(otherStyle)
    }))
    renderAt('/11/document_styles/702/edit_general')
    expect(await screen.findByRole('heading', { name: 'Change name and sender address', level: 1 })).toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toHaveValue('The Best')
    await userEvent.selectOptions(screen.getByLabelText('Sender address'), '501')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent('/11/document_styles/702/edit/document'))
    expect(sent.document_style).toEqual({ name: 'The Best', identity_id: '501', default: false })
  })
})
