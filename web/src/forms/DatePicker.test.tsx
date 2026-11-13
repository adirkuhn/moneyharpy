import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { DateInput } from './DatePicker'
import { calendarMonth, isDay, shiftDay } from './calendar'

function Field({ initial = '2026-11-13', min, max, onPick }: { initial?: string; min?: string; max?: string; onPick?: (d: string) => void }) {
  const [value, setValue] = useState(initial)
  return (
    <>
      <DateInput label="Start date" value={value} min={min} max={max} today="2026-11-13" onChange={(d) => { setValue(d); onPick?.(d) }} />
      <button type="button">Elsewhere</button>
    </>
  )
}

const calendar = () => screen.getByRole('dialog', { name: 'Choose a date' })
const grid = () => within(calendar()).getByRole('grid')

describe('calendar arithmetic', () => {
  it('knows a real day from anything else', () => {
    expect(isDay('2026-11-13')).toBe(true)
    expect(isDay('2026-02-30')).toBe(false)
    expect(isDay('2026-11-1')).toBe(false)
    expect(isDay('')).toBe(false)
  })

  it('lays a month out in weeks starting on Monday', () => {
    const weeks = calendarMonth(2026, 11)
    // 1 November 2026 is a Sunday: six empty places before it.
    expect(weeks[0]).toEqual([null, null, null, null, null, null, '2026-11-01'])
    expect(weeks.at(-1)?.[0]).toBe('2026-11-30')
    expect(weeks).toHaveLength(6)
  })

  it('moves days across months and years', () => {
    expect(shiftDay('2026-12-31', 1)).toBe('2027-01-01')
    expect(shiftDay('2026-03-01', -1)).toBe('2026-02-28')
  })
})

describe('DatePicker', () => {
  it('is a text field in YYYY-MM-DD with a calendar button beside it', () => {
    render(<Field />)
    const input = screen.getByLabelText('Start date')
    expect(input).toHaveAttribute('type', 'text')
    expect(input).toHaveAttribute('placeholder', 'YYYY-MM-DD')
    expect(input).toHaveValue('2026-11-13')
    expect(screen.getByRole('button', { name: 'Calendar' })).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('opens the calendar from its button on the month of the date, the day selected', async () => {
    const user = userEvent.setup()
    render(<Field />)
    await user.click(screen.getByRole('button', { name: 'Calendar' }))
    expect(within(calendar()).getByText('November 2026')).toBeInTheDocument()
    const headers = within(grid()).getAllByRole('columnheader').map((h) => h.textContent)
    expect(headers).toEqual(['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su'])
    const day = within(grid()).getByRole('gridcell', { name: 'Friday 13 November 2026' })
    expect(day).toHaveAttribute('aria-selected', 'true')
    expect(day).toHaveAttribute('aria-current', 'date')
    expect(screen.getByLabelText('Start date')).toHaveFocus()
  })

  it('opens when the field is clicked and closes on a second click of the button', async () => {
    const user = userEvent.setup()
    render(<Field />)
    await user.click(screen.getByLabelText('Start date'))
    expect(calendar()).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Calendar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('pages through months, the previous one closed while it lies before the minimum', async () => {
    const user = userEvent.setup()
    render(<Field min="2026-11-13" />)
    await user.click(screen.getByRole('button', { name: 'Calendar' }))
    const previous = within(calendar()).getByRole('button', { name: 'Previous month' })
    expect(previous).toHaveAttribute('aria-disabled', 'true')
    await user.click(previous)
    expect(within(calendar()).getByText('November 2026')).toBeInTheDocument()
    expect(within(grid()).getByRole('gridcell', { name: 'Thursday 12 November 2026' })).toHaveAttribute('aria-disabled', 'true')
    await user.click(within(calendar()).getByRole('button', { name: 'Next month' }))
    expect(within(calendar()).getByText('December 2026')).toBeInTheDocument()
    expect(previous).not.toHaveAttribute('aria-disabled')
    await user.click(previous)
    expect(within(calendar()).getByText('November 2026')).toBeInTheDocument()
  })

  it('closes the next month after the maximum', async () => {
    const user = userEvent.setup()
    render(<Field initial="2026-11-02" max="2026-11-20" />)
    await user.click(screen.getByRole('button', { name: 'Calendar' }))
    expect(within(calendar()).getByRole('button', { name: 'Next month' })).toHaveAttribute('aria-disabled', 'true')
    expect(within(grid()).getByRole('gridcell', { name: 'Saturday 21 November 2026' })).toHaveAttribute('aria-disabled', 'true')
  })

  it('takes a picked day into the field and closes', async () => {
    const user = userEvent.setup()
    const onPick = vi.fn()
    render(<Field onPick={onPick} />)
    await user.click(screen.getByRole('button', { name: 'Calendar' }))
    await user.click(within(grid()).getByRole('gridcell', { name: 'Thursday 26 November 2026' }))
    expect(onPick).toHaveBeenLastCalledWith('2026-11-26')
    expect(screen.getByLabelText('Start date')).toHaveValue('2026-11-26')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('does not take a day outside the allowed range', async () => {
    const user = userEvent.setup()
    const onPick = vi.fn()
    render(<Field min="2026-11-13" onPick={onPick} />)
    await user.click(screen.getByRole('button', { name: 'Calendar' }))
    await user.click(within(grid()).getByRole('gridcell', { name: 'Monday 2 November 2026' }))
    expect(onPick).not.toHaveBeenCalled()
    expect(calendar()).toBeInTheDocument()
  })

  it('moves between days with the arrow keys and picks with Enter', async () => {
    const user = userEvent.setup()
    render(<Field />)
    await user.click(screen.getByRole('button', { name: 'Calendar' }))
    await user.keyboard('{ArrowDown}')
    expect(within(grid()).getByRole('gridcell', { name: 'Friday 13 November 2026' })).toHaveFocus()
    await user.keyboard('{ArrowRight}{ArrowDown}')
    expect(within(grid()).getByRole('gridcell', { name: 'Saturday 21 November 2026' })).toHaveFocus()
    await user.keyboard('{ArrowDown}{ArrowDown}')
    // Past the end of the month the calendar follows into December.
    expect(within(calendar()).getByText('December 2026')).toBeInTheDocument()
    expect(within(grid()).getByRole('gridcell', { name: 'Saturday 5 December 2026' })).toHaveFocus()
    await user.keyboard('{ArrowLeft}{Enter}')
    expect(screen.getByLabelText('Start date')).toHaveValue('2026-12-04')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Start date')).toHaveFocus()
  })

  it('stays closed while a date is only typed, and opens with the down arrow', async () => {
    const user = userEvent.setup()
    render(<Field />)
    const input = screen.getByLabelText('Start date')
    input.focus()
    await user.keyboard('{Backspace}4')
    expect(input).toHaveValue('2026-11-14')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await user.keyboard('{ArrowDown}')
    expect(within(grid()).getByRole('gridcell', { name: 'Saturday 14 November 2026' })).toHaveAttribute('aria-selected', 'true')
  })

  it('closes with Escape', async () => {
    const user = userEvent.setup()
    render(<Field />)
    await user.click(screen.getByLabelText('Start date'))
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Start date')).toHaveFocus()
  })

  it('closes when something else is clicked', async () => {
    const user = userEvent.setup()
    render(<Field />)
    await user.click(screen.getByLabelText('Start date'))
    await user.click(screen.getByRole('button', { name: 'Elsewhere' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('takes a typed date and shows its month', async () => {
    const user = userEvent.setup()
    const onPick = vi.fn()
    render(<Field onPick={onPick} />)
    const input = screen.getByLabelText('Start date')
    await user.clear(input)
    await user.type(input, '2027-03-05')
    expect(input).toHaveValue('2027-03-05')
    expect(onPick).toHaveBeenLastCalledWith('2027-03-05')
    expect(within(calendar()).getByText('March 2027')).toBeInTheDocument()
    expect(within(grid()).getByRole('gridcell', { name: 'Friday 5 March 2027' })).toHaveAttribute('aria-selected', 'true')
  })

  it('keeps letters out of the field', async () => {
    const user = userEvent.setup()
    render(<Field initial="" />)
    await user.type(screen.getByLabelText('Start date'), '20a26-1x1')
    expect(screen.getByLabelText('Start date')).toHaveValue('2026-11')
  })

  it('cannot be opened while disabled', async () => {
    const user = userEvent.setup()
    render(<DateInput label="Invoice date" value="" disabled onChange={() => {}} />)
    expect(screen.getByLabelText('Invoice date')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Calendar' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Calendar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('names the months and weekdays in Dutch', async () => {
    const user = userEvent.setup()
    await i18n.changeLanguage('nl')
    render(<Field />)
    await user.click(screen.getByRole('button', { name: 'Kalender' }))
    const dialog = screen.getByRole('dialog', { name: 'Kies een datum' })
    expect(within(dialog).getByText('november 2026')).toBeInTheDocument()
    expect(within(dialog).getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['ma', 'di', 'wo', 'do', 'vr', 'za', 'zo'])
    await i18n.changeLanguage('en')
  })
})
