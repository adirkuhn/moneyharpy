import { type ChangeEvent, type KeyboardEvent, type ReactNode, useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ListIcon } from '../layout/icons'
import {
  addMonths, calendarMonth, clampDay, firstOf, isDay, lastOf, type Month, monthOf, noonOf, shiftDay, shiftMonth, todayDay, weekdayOf,
} from './calendar'
import { type ControlProps, FormRow, type Width } from './Form'
import './datePicker.css'

export type DatePickerProps = Partial<ControlProps> & {
  /** The day in YYYY-MM-DD, or whatever has been typed so far. */
  value: string
  onChange: (value: string) => void
  /** The first and last day that can be picked, in YYYY-MM-DD. */
  min?: string
  max?: string
  disabled?: boolean
  required?: boolean
  name?: string
  'aria-labelledby'?: string
  /** The day marked as today; the browser's own day when left out. */
  today?: string
}

const weekdays = [0, 1, 2, 3, 4, 5, 6]
// 2 November 2026 is a Monday: the week from it names the weekdays.
const aMonday = '2026-11-02'

/**
 * Moneybird's date field: a text field in YYYY-MM-DD with a calendar
 * button, which opens a month to pick a day from under the field. A click
 * on the field or the button, or the down arrow, opens it; typing alone
 * does not, so a typed date leaves the fields below uncovered.
 */
export function DatePicker({ value, onChange, min, max, disabled = false, today: todayProp, ...input }: DatePickerProps) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const today = todayProp ?? todayDay()
  const [open, setOpen] = useState(false)
  const [view, setView] = useState<Month>(() => monthOf(today))
  const [active, setActive] = useState(today)
  const [inGrid, setInGrid] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const field = useRef<HTMLInputElement>(null)

  const allowed = (day: string) => (!min || day >= min) && (!max || day <= max)

  const show = () => {
    if (disabled) {
      return
    }
    const day = isDay(value) ? value : clampDay(today, min, max)
    setView(monthOf(day))
    setActive(clampDay(day, min, max))
    setInGrid(false)
    setOpen(true)
  }

  const hide = (refocus: boolean) => {
    setOpen(false)
    setInGrid(false)
    if (refocus) {
      field.current?.focus()
    }
  }

  const pick = (day: string) => {
    if (!allowed(day)) {
      return
    }
    onChange(day)
    hide(true)
  }

  const moveTo = (day: string) => {
    const next = clampDay(day, min, max)
    setActive(next)
    setView(monthOf(next))
  }

  // Clicking anywhere else closes the calendar.
  useEffect(() => {
    if (!open) {
      return
    }
    const outside = (e: MouseEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) {
        hide(false)
      }
    }
    document.addEventListener('mousedown', outside)
    return () => document.removeEventListener('mousedown', outside)
  }, [open])

  // Keyboard moves through the days carry the focus along.
  useEffect(() => {
    if (open && inGrid) {
      root.current?.querySelector<HTMLElement>(`[data-day="${active}"]`)?.focus()
    }
  }, [open, inGrid, active, view])

  const type = (e: ChangeEvent<HTMLInputElement>) => {
    // Only digits and dashes, as Moneybird's field takes.
    const typed = e.target.value.replace(/[^\d-]/g, '').slice(0, 10)
    onChange(typed)
    if (isDay(typed)) {
      setView(monthOf(typed))
      setActive(clampDay(typed, min, max))
    }
  }

  const fieldKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (open) {
        setInGrid(true)
      } else {
        show()
      }
    }
  }

  const dayKey = (e: KeyboardEvent<HTMLElement>) => {
    const moves: Record<string, () => string> = {
      ArrowLeft: () => shiftDay(active, -1),
      ArrowRight: () => shiftDay(active, 1),
      ArrowUp: () => shiftDay(active, -7),
      ArrowDown: () => shiftDay(active, 7),
      Home: () => shiftDay(active, -weekdayOf(active)),
      End: () => shiftDay(active, 6 - weekdayOf(active)),
      PageUp: () => shiftMonth(active, -1),
      PageDown: () => shiftMonth(active, 1),
    }
    const move = moves[e.key]
    if (move) {
      e.preventDefault()
      moveTo(move())
    }
  }

  const escape = (e: KeyboardEvent) => {
    if (e.key === 'Escape' && open) {
      // Only the calendar closes, not a dialog the field sits in.
      e.preventDefault()
      e.stopPropagation()
      hide(true)
    }
  }

  const leave = (e: React.FocusEvent) => {
    if (open && e.relatedTarget && !root.current?.contains(e.relatedTarget as Node)) {
      hide(false)
    }
  }

  const language = i18n.language
  const monthName = new Intl.DateTimeFormat(language, { month: 'long' }).format(noonOf(firstOf(view)))
  const weekdayShort = new Intl.DateTimeFormat(language, { weekday: 'short' })
  const dayName = (day: string) => {
    const date = noonOf(day)
    const part = (options: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(language, options).format(date)
    return `${part({ weekday: 'long' })} ${date.getDate()} ${part({ month: 'long' })} ${date.getFullYear()}`
  }

  const previousClosed = !!min && lastOf(addMonths(view, -1)) < min
  const nextClosed = !!max && firstOf(addMonths(view, 1)) > max
  const weeks = calendarMonth(view.year, view.month)
  const inView = (day: string) => day >= firstOf(view) && day <= lastOf(view)
  // The one day the Tab key reaches: the active one, or the month's first allowed day.
  const tabDay = inView(active) ? active : (weeks.flat().find((d): d is string => !!d && allowed(d)) ?? firstOf(view))

  const cycler = (label: string, closed: boolean, n: number, icon: ReactNode) => (
    <button
      type="button"
      className={`date-picker__cycler${closed ? ' date-picker__cycler--disabled' : ''}`}
      aria-label={label}
      {...(closed ? { 'aria-disabled': true } : {})}
      onClick={() => !closed && setView(addMonths(view, n))}
    >
      {icon}
    </button>
  )

  return (
    <div className="date-picker" ref={root} onKeyDown={escape} onBlur={leave}>
      <input
        ref={field}
        type="text"
        className="date-picker__input"
        placeholder="YYYY-MM-DD"
        autoComplete="off"
        maxLength={10}
        disabled={disabled}
        {...input}
        value={value}
        onChange={type}
        onClick={() => !open && show()}
        onKeyDown={fieldKey}
      />
      <button
        type="button"
        className="date-picker__icon"
        aria-label={t('datePicker.calendar')}
        aria-haspopup="dialog"
        aria-expanded={open}
        {...(open ? { 'aria-controls': `${id}-calendar` } : {})}
        disabled={disabled}
        tabIndex={-1}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (open) {
            hide(false)
            return
          }
          show()
          field.current?.focus()
        }}
      >
        <ListIcon kind="calendar" />
      </button>
      {open && (
        <div className="date-picker__calendar" id={`${id}-calendar`} role="dialog" aria-label={t('datePicker.choose')}>
          <div className="date-picker__header">
            {cycler(t('datePicker.previousMonth'), previousClosed, -1, <Chevron left />)}
            <span className="date-picker__month" id={`${id}-month`} aria-live="polite">{`${monthName} ${view.year}`}</span>
            {cycler(t('datePicker.nextMonth'), nextClosed, 1, <Chevron />)}
          </div>
          <div className="date-picker__body" role="grid" aria-labelledby={`${id}-month`}>
            <div className="date-picker__week" role="row">
              {weekdays.map((w) => {
                const day = shiftDay(aMonday, w)
                return (
                  <span key={w} className="date-picker__weekday" role="columnheader" title={new Intl.DateTimeFormat(language, { weekday: 'long' }).format(noonOf(day))}>
                    {weekdayShort.format(noonOf(day)).replace('.', '').slice(0, 2)}
                  </span>
                )
              })}
            </div>
            {weeks.map((week) => (
              <div key={week.find((d) => d) ?? ''} className="date-picker__week" role="row">
                {week.map((day, i) => {
                  if (!day) {
                    return <span key={`empty-${i}`} className="date-picker__day date-picker__day--empty" role="gridcell" />
                  }
                  const closed = !allowed(day)
                  const selected = day === value
                  const classes = ['date-picker__day']
                  if (selected) classes.push('date-picker__day--selected')
                  if (day === today) classes.push('date-picker__day--today')
                  if (closed) classes.push('date-picker__day--disabled')
                  return (
                    <button
                      key={day}
                      type="button"
                      role="gridcell"
                      data-day={day}
                      className={classes.join(' ')}
                      aria-label={dayName(day)}
                      aria-selected={selected}
                      {...(day === today ? { 'aria-current': 'date' as const } : {})}
                      {...(closed ? { 'aria-disabled': true } : {})}
                      tabIndex={day === tabDay ? 0 : -1}
                      onClick={() => pick(day)}
                      onFocus={() => {
                        setActive(day)
                        setInGrid(true)
                      }}
                      onKeyDown={dayKey}
                    >
                      {Number(day.slice(8))}
                    </button>
                  )
                })}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function Chevron({ left = false }: { left?: boolean }) {
  return (
    <svg viewBox="0 0 24 24" width="1em" height="1em" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <path d={left ? 'm14.500 6-6 6 6 6' : 'm9.500 6 6 6-6 6'} />
    </svg>
  )
}

type DateInputProps = Omit<DatePickerProps, keyof ControlProps> & {
  label: string
  errors?: string[]
  help?: ReactNode
  width?: Width
  action?: ReactNode
  labelExtra?: ReactNode
}

/** A labelled date field, as a form row. */
export function DateInput({ label, errors, help, width = 'medium', action, labelExtra, ...picker }: DateInputProps) {
  return (
    <FormRow label={label} errors={errors} help={help} width={width} action={action} labelExtra={labelExtra}>
      {({ labelId: _, ...control }) => <DatePicker {...picker} {...control} />}
    </FormRow>
  )
}
