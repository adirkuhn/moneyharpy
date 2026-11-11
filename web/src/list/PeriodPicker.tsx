import { useState } from 'react'
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

/**
 * Picks a period over two years by month, quarter or year, after Moneybird's
 * range slider: a first choice picks that span, a second one widens it to a
 * range from the one to the other.
 */
export function PeriodPicker({ value, onChange, today = new Date() }: Props) {
  const { t, i18n } = useTranslation()
  const thisYear = today.getFullYear()
  const [first, setFirst] = useState(() => Math.min(value ? yearOfMonth(value.to) : thisYear, thisYear) - 1)
  const [anchor, setAnchor] = useState<MonthRange | null>(null)
  const start = monthOf(first, 0)

  const months: Cell[] = Array.from({ length: 24 }, (_, i) => {
    const date = new Date(first + Math.floor(i / 12), i % 12, 1)
    return {
      key: `m${i}`, from: start + i, to: start + i,
      text: date.toLocaleDateString(i18n.language, { month: 'short' }),
      label: date.toLocaleDateString(i18n.language, { month: 'long', year: 'numeric' }),
    }
  })
  const quarters: Cell[] = Array.from({ length: 8 }, (_, i) => ({
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

  const row = (name: string, cells: Cell[], span: number) => (
    <div className="period-picker__row" role="group" aria-label={name}>
      {cells.map((cell) => (
        <button
          key={cell.key}
          type="button"
          className={`period-picker__cell${chosen(cell) ? ' is-chosen' : ''}`}
          style={{ gridColumn: `span ${span}` }}
          aria-label={cell.label}
          aria-pressed={chosen(cell)}
          onClick={() => choose(cell)}
        >
          {cell.text}
        </button>
      ))}
    </div>
  )

  return (
    <fieldset className="filter-field period-picker">
      <legend className="filter-field__label">{t('list.period')}</legend>
      <div className="period-picker__bar">
        <button type="button" className="icon-btn" aria-label={t('list.earlierYears')} title={t('list.earlierYears')} onClick={() => setFirst(first - 1)}>
          <ArrowLeftIcon />
        </button>
        <button type="button" className="icon-btn" aria-label={t('list.laterYears')} title={t('list.laterYears')} disabled={first + 1 >= thisYear} onClick={() => setFirst(first + 1)}>
          <ChevronIcon />
        </button>
        <button
          type="button"
          className={`btn btn--small btn--secondary${value === null ? ' is-active' : ''}`}
          aria-pressed={value === null}
          onClick={() => {
            onChange(null)
            setAnchor(null)
          }}
        >
          {t('list.allPeriods')}
        </button>
        <p className="period-picker__chosen" role="status">{value ? describePeriod(value, i18n.language) : t('list.allPeriods')}</p>
      </div>
      <div className="period-picker__grid">
        {row(t('list.months'), months, 1)}
        {row(t('list.quarters'), quarters, 3)}
        {row(t('list.years'), years, 12)}
      </div>
      <p className="hint">{t('list.periodHelp')}</p>
    </fieldset>
  )
}
