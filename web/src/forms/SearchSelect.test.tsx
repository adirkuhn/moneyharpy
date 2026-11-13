import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { matches, type SearchGroup, SearchSelect } from './SearchSelect'

const groups: SearchGroup[] = [
  { options: [{ value: '', label: 'Uncategorized' }] },
  {
    label: 'Direct costs',
    options: [
      { value: '1', label: 'Purchases (7000)', code: '7000' },
      { value: '2', label: 'Café supplies', depth: 1 },
    ],
  },
  { label: 'Expenses', options: [{ value: '3', label: 'Office costs (4100)', code: '4100' }, { value: '4', label: 'Rent' }] },
]

function Field({ onAdd, pinned, initial = '' }: { onAdd?: () => void; pinned?: SearchGroup; initial?: string }) {
  const [value, setValue] = useState(initial)
  return (
    <>
      <SearchSelect
        label="Category"
        searchLabel="Find category"
        emptyLabel="No category found"
        value={value}
        groups={groups}
        pinned={pinned}
        add={onAdd && { label: 'Add Category...', onAdd }}
        onChange={setValue}
      />
      <button type="button">Elsewhere</button>
    </>
  )
}

const trigger = () => screen.getByRole('combobox', { name: 'Category' })
const search = () => screen.getByRole('searchbox', { name: 'Find category' })
const listbox = () => screen.getByRole('listbox', { name: 'Category' })
const optionNames = () => within(listbox()).queryAllByRole('option').map((o) => o.textContent)

describe('matching', () => {
  it('ignores case and accents, and finds codes', () => {
    expect(matches({ value: '2', label: 'Café supplies' }, 'cafe')).toBe(true)
    expect(matches({ value: '2', label: 'Cafe supplies' }, 'CAFÉ')).toBe(true)
    expect(matches({ value: '3', label: 'Office costs', code: '4100' }, '410')).toBe(true)
    expect(matches({ value: '4', label: 'Rent' }, 'office')).toBe(false)
  })
})

describe('the searchable select', () => {
  it('shows the chosen value closed, and opens on a search with every option in its group', async () => {
    render(<Field initial="3" />)
    expect(trigger()).toHaveTextContent('Office costs (4100)')
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
    await userEvent.click(trigger())
    expect(trigger()).toHaveAttribute('aria-expanded', 'true')
    expect(search()).toHaveFocus()
    expect(search()).toHaveAttribute('placeholder', 'Find category')
    expect(within(listbox()).getByRole('group', { name: 'Direct costs' })).toBeInTheDocument()
    expect(within(within(listbox()).getByRole('group', { name: 'Expenses' })).getByRole('option', { name: 'Office costs (4100)' })).toHaveAttribute('aria-selected', 'true')
    expect(optionNames()).toEqual(['Uncategorized', 'Purchases (7000)', 'Café supplies', 'Office costs (4100)', 'Rent'])
    // The chosen one is where the keyboard starts.
    expect(search()).toHaveAttribute('aria-activedescendant', within(listbox()).getByRole('option', { name: 'Office costs (4100)' }).id)
  })

  it('narrows the options as you type, keeping the heading of a group with a match', async () => {
    render(<Field />)
    await userEvent.click(trigger())
    await userEvent.type(search(), 'cafe')
    expect(optionNames()).toEqual(['Café supplies'])
    expect(within(listbox()).getByRole('group', { name: 'Direct costs' })).toBeInTheDocument()
    expect(within(listbox()).queryByRole('group', { name: 'Expenses' })).toBeNull()
    await userEvent.clear(search())
    await userEvent.type(search(), '4100')
    expect(optionNames()).toEqual(['Office costs (4100)'])
    await userEvent.clear(search())
    await userEvent.type(search(), 'spaceship')
    expect(optionNames()).toEqual([])
    expect(within(listbox()).getByText('No category found')).toBeInTheDocument()
  })

  it('is worked with the keyboard: arrows, Home, End, Enter and Escape', async () => {
    render(<Field initial="" />)
    trigger().focus()
    await userEvent.keyboard('{ArrowDown}')
    expect(search()).toHaveFocus()
    const active = () => document.getElementById(search().getAttribute('aria-activedescendant') ?? '')?.textContent
    expect(active()).toBe('Uncategorized')
    await userEvent.keyboard('{ArrowDown}{ArrowDown}')
    expect(active()).toBe('Café supplies')
    await userEvent.keyboard('{End}')
    expect(active()).toBe('Rent')
    await userEvent.keyboard('{Home}{ArrowDown}{ArrowUp}{ArrowUp}')
    expect(active()).toBe('Uncategorized')
    await userEvent.keyboard('{End}{Enter}')
    expect(trigger()).toHaveTextContent('Rent')
    expect(screen.queryByRole('listbox')).toBeNull()
    expect(trigger()).toHaveFocus()

    await userEvent.keyboard('{Enter}')
    expect(listbox()).toBeInTheDocument()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('listbox')).toBeNull()
    expect(trigger()).toHaveFocus()
    expect(trigger()).toHaveTextContent('Rent')
  })

  it('starts a search when you type on the closed select', async () => {
    render(<Field />)
    trigger().focus()
    await userEvent.keyboard('of')
    expect(search()).toHaveValue('of')
    expect(optionNames()).toEqual(['Office costs (4100)'])
    await userEvent.keyboard('{Enter}')
    expect(trigger()).toHaveTextContent('Office costs (4100)')
  })

  it('closes on a click elsewhere without choosing', async () => {
    render(<Field initial="1" />)
    await userEvent.click(trigger())
    await userEvent.click(screen.getByRole('button', { name: 'Elsewhere' }))
    expect(screen.queryByRole('listbox')).toBeNull()
    expect(trigger()).toHaveTextContent('Purchases (7000)')
  })

  it('chooses an option clicked', async () => {
    render(<Field />)
    await userEvent.click(trigger())
    await userEvent.click(screen.getByRole('option', { name: 'Rent' }))
    expect(trigger()).toHaveTextContent('Rent')
  })

  it('offers the most used options first, until you search', async () => {
    render(<Field pinned={{ label: 'Most used', options: [{ value: '4', label: 'Rent' }, { value: '1', label: 'Purchases (7000)' }] }} />)
    await userEvent.click(trigger())
    expect(within(within(listbox()).getByRole('group', { name: 'Most used' })).getAllByRole('option').map((o) => o.textContent)).toEqual(['Rent', 'Purchases (7000)'])
    expect(optionNames()[0]).toBe('Rent')
    await userEvent.click(within(within(listbox()).getByRole('group', { name: 'Most used' })).getByRole('option', { name: 'Rent' }))
    expect(trigger()).toHaveTextContent('Rent')
    await userEvent.click(trigger())
    await userEvent.type(search(), 'r')
    expect(within(listbox()).queryByRole('group', { name: 'Most used' })).toBeNull()
    expect(optionNames()).toEqual(['Uncategorized', 'Purchases (7000)', 'Rent'])
  })

  it('ends with a row to add one, also when nothing is found', async () => {
    const onAdd = vi.fn()
    render(<Field onAdd={onAdd} />)
    await userEvent.click(trigger())
    await userEvent.type(search(), 'spaceship')
    expect(within(listbox()).getByText('No category found')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('option', { name: 'Add Category...' }))
    expect(onAdd).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('listbox')).toBeNull()

    await userEvent.click(trigger())
    await userEvent.keyboard('{End}{Enter}')
    expect(onAdd).toHaveBeenCalledTimes(2)
  })
})
