import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { renderAt } from '../test/render'
import { administration, listing, server, signedIn, user } from '../test/server'

const harpy = administration({ id: '11', name: 'Harpy B.V.' })

describe('top navigation', () => {
  it('switches between administrations', async () => {
    server.use(signedIn(), listing([harpy]))
    renderAt('/administrations')
    await userEvent.click(await screen.findByRole('button', { name: 'Select administration' }))
    const menu = await screen.findByRole('navigation', { name: 'Your administrations' })
    expect(await within(menu).findByRole('link', { name: 'Harpy B.V.' })).toHaveAttribute('href', '/11')
    expect(within(menu).getByRole('link', { name: 'All administrations' })).toHaveAttribute('href', '/administrations')
    expect(within(menu).getByRole('link', { name: 'New administration' })).toHaveAttribute('href', '/onboarding/company/create_administration?origin=onboarding')
  })

  it('closes a menu with Escape', async () => {
    server.use(signedIn(), listing([harpy]))
    renderAt('/administrations')
    await userEvent.click(await screen.findByRole('button', { name: 'Select administration' }))
    await screen.findByRole('navigation', { name: 'Your administrations' })
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('navigation', { name: 'Your administrations' })).toBeNull()
  })

  it('remembers the chosen theme', async () => {
    server.use(signedIn(), listing([]))
    renderAt('/administrations')
    await userEvent.click(await screen.findByRole('button', { name: user.name }))
    await userEvent.selectOptions(screen.getByLabelText('Theme'), 'dark')
    expect(document.documentElement).toHaveAttribute('data-theme', 'dark')
    expect(localStorage.getItem('moneyharpy.theme')).toBe('dark')
  })

  it('switches the language of the whole app', async () => {
    server.use(signedIn(), listing([]))
    renderAt('/administrations')
    await userEvent.click(await screen.findByRole('button', { name: user.name }))
    await userEvent.selectOptions(screen.getByLabelText('Language'), 'nl')
    expect(await screen.findByRole('heading', { name: 'Kies een administratie' })).toBeInTheDocument()
    expect(document.documentElement).toHaveAttribute('lang', 'nl')
    expect(localStorage.getItem('moneyharpy.language')).toBe('nl')
  })

  it('links to the API documentation', async () => {
    server.use(signedIn(), listing([]))
    renderAt('/administrations')
    await userEvent.click(await screen.findByRole('button', { name: user.name }))
    expect(screen.getByRole('link', { name: 'API documentation' })).toHaveAttribute('href', '/docs/api/')
  })

  it('shows instance settings to instance administrators only', async () => {
    server.use(signedIn(), listing([]))
    renderAt('/administrations')
    await userEvent.click(await screen.findByRole('button', { name: user.name }))
    expect(screen.queryByRole('link', { name: 'Instance settings' })).toBeNull()
  })
})
