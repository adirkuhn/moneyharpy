import { type KeyboardEvent, type PointerEvent, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeftIcon, ChevronIcon } from '../layout/icons'
import { describePeriod, monthOf, type MonthRange, yearOfMonth } from './period'

type Props = {
  /** The months chosen; null for any period. */
  value: MonthRange | null
  onChange: (range: MonthRange | null) => void
  today?: Date
}

type Cell = MonthRange & { key: string; text: string; label: string }
type End = 'start' | 'end'

/** The months shown: two years. */
const shown = 24

const clamp = (n: number, min: number, max: number) => Math.min(Math.max(n, min), max)

/**
 * Picks a period over two years by month, quarter or year, after Moneybird's
 * range slider. A first choice picks that span, a second one widens it to a
 * range from the one to the other. The period shows as a band with a handle
 * at each end, dragged with a pointer or moved a month at a time with the
 * arrow keys.
 */
export function PeriodPicker({ value, onChange, today = new Date() }: Props) {
  const { t, i18n } = useTranslation()
  const hint = useId()
  const thisYear = today.getFullYear()
  const [first, setFirst] = useState(() => Math.min(value ? yearOfMonth(value.to) : thisYear, thisYear) - 1)
  const [anchor, setAnchor] = useState<MonthRange | null>(null)
  const track = useRef<HTMLDivElement>(null)
  const dragging = useRef<End | null>(null)
  const start = monthOf(first, 0)

  const monthLabel = (m: number) => new Date(yearOfMonth(m), m % 12, 1).toLocaleDateString(i18n.language, { month: 'long', year: 'numeric' })
  const months: Cell[] = Array.from({ length: shown }, (_, i) => ({
    key: `m${i}`, from: start + i, to: start + i,
    text: new Date(first, i, 1).toLocaleDateString(i18n.language, { month: 'short' }),
    label: monthLabel(start + i),
  }))
  const quarters: Cell[] = Array.from({ length: shown / 3 }, (_, i) => ({
    key: `q${i}`, from: start + 3 * i, to: start + 3 * i + 2,
    text: t('list.quarterShort', { n: (i % 4) + 1 }),
    label: t('list.quarter', { n: (i % 4) + 1, year: first + Math.floor(i / 4) }),
  }))
  const years: Cell[] = [0, 1].map((i) => ({ key: `y${i}`, from: start + 12 * i, to: start + 12 * i + 11, text: String(first + i), label: String(first + i) }))

  const choose = (cell: Cell) => {
    if (anchor) {
      onChange({ from: Math.min(anchor.from, cell.from), to: Math.max(anchor.to, cell.to) })
      setAnchor(null)
    } else {
      onChange({ from: cell.from, to: cell.to })
      setAnchor({ from: cell.from, to: cell.to })
    }
  }
  const chosen = (cell: Cell) => value !== null && cell.from >= value.from && cell.to <= value.to

  // Where the period lies among the months shown, counted from the first one.
  const from = value ? value.from - start : 0
  const to = value ? value.to - start : -1
  const bandFrom = Math.max(from, 0)
  const bandTo = Math.min(to, shown - 1)

  /** Moves one end of the period to a month shown, keeping the start before the end. */
  const move = (end: End, index: number) => {
    if (!value) {
      return
    }
    setAnchor(null)
    if (end === 'start') {
      onChange({ from: start + clamp(index, 0, Math.min(to, shown - 1)), to: value.to })
    } else {
      onChange({ from: value.from, to: start + clamp(index, Math.max(from, 0), shown - 1) })
    }
  }
  const onKeyDown = (end: End, e: KeyboardEvent) => {
    const at = end === 'start' ? from : to
    const steps: Record<string, number> = { ArrowLeft: at - 1, ArrowDown: at - 1, ArrowRight: at + 1, ArrowUp: at + 1, Home: 0, End: shown - 1 }
    const next = steps[e.key]
    if (next !== undefined) {
      e.preventDefault()
      move(end, next)
    }
  }
  // A handle sits on the edge between two months: the start before its month, the end after it.
  const onPointerDown = (end: End, e: PointerEvent<HTMLElement>) => {
    e.preventDefault()
    e.currentTarget.setPointerCapture?.(e.pointerId)
    e.currentTarget.focus()
    dragging.current = end
  }
  const onPointerMove = (end: End, e: PointerEvent) => {
    const rect = track.current?.getBoundingClientRect()
    if (dragging.current !== end || !rect || rect.width === 0) {
      return
    }
    const edge = Math.round(((e.clientX - rect.left) / rect.width) * shown)
    move(end, end === 'start' ? edge : edge - 1)
  }
  const stopDragging = () => {
    dragging.current = null
  }

  const handle = (end: End, at: number, min: number, max: number) => (
    <span
      role="slider"
      tabIndex={0}
      className={`period-picker__handle period-picker__handle--${end}`}
      style={{ left: `${((end === 'start' ? at : at + 1) / shown) * 100}%` }}
      aria-label={t(end === 'start' ? 'list.periodStart' : 'list.periodEnd')}
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={at}
      aria-valuetext={monthLabel(start + at)}
      onKeyDown={(e) => onKeyDown(end, e)}
      onPointerDown={(e) => onPointerDown(end, e)}
      onPointerMove={(e) => onPointerMove(end, e)}
      onPointerUp={stopDragging}
      onPointerCancel={stopDragging}
    />
  )

  const cellButton = (cell: Cell) => (
    <button
      key={cell.key}
      type="button"
      className={`period-picker__cell${chosen(cell) ? ' is-chosen' : ''}`}
      aria-label={cell.label}
      aria-pressed={chosen(cell)}
      onClick={() => choose(cell)}
    >
      {cell.text}
    </button>
  )

  return (
    <fieldset className="filter-field period-picker" aria-describedby={hint}>
      <legend className="sr-only">{t('list.period')}</legend>
      <p id={hint} className="sr-only">{t('list.periodHelp')}</p>
      <div className="period-picker__scroll">
        <div className="period-picker__track" ref={track}>
          <div className="period-picker__row period-picker__row--months" role="group" aria-label={t('list.months')}>{months.map(cellButton)}</div>
          <div className="period-picker__row period-picker__row--quarters" role="group" aria-label={t('list.quarters')}>{quarters.map(cellButton)}</div>
          <div className="period-picker__row period-picker__row--years" role="group" aria-label={t('list.years')}>
            {years.map((cell, i) => (
              <div key={cell.key} className="period-picker__year">
                {cellButton(cell)}
                {i === 0 ? (
                  <button type="button" className="period-picker__step period-picker__step--earlier" aria-label={t('list.earlierYears')} title={t('list.earlierYears')} onClick={() => setFirst(first - 1)}>
                    <ArrowLeftIcon />
                  </button>
                ) : (
                  <button type="button" className="period-picker__step period-picker__step--later" aria-label={t('list.laterYears')} title={t('list.laterYears')} disabled={first + 1 >= thisYear} onClick={() => setFirst(first + 1)}>
                    <ChevronIcon />
                  </button>
                )}
              </div>
            ))}
          </div>
          {bandFrom <= bandTo && (
            <span
              className="period-picker__band"
              aria-hidden="true"
              style={{ left: `${(bandFrom / shown) * 100}%`, width: `${((bandTo - bandFrom + 1) / shown) * 100}%` }}
            />
          )}
          {value && from >= 0 && from < shown && handle('start', from, 0, Math.min(to, shown - 1))}
          {value && to >= 0 && to < shown && handle('end', to, Math.max(from, 0), shown - 1)}
        </div>
      </div>
      <p className="sr-only" role="status">{value ? describePeriod(value, i18n.language) : t('list.allPeriods')}</p>
    </fieldset>
  )
}
