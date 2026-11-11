import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AdvancedFilter, FilterPanel } from './FilterPanel'
import { describePeriod, parsePeriod, periodValue } from './period'
import { PeriodPicker } from './PeriodPicker'

/** The period that stands for no period at all, in the address. */
export const allPeriods = 'all'

type Props = {
  /** The period shown now: a named one such as this_month, a range such as 202401..202403, or "all". */
  period: string
  /** The panel's name, such as "Report filters". */
  title: string
  /** The named periods offered as quick filters, with their names. */
  quick: { value: string; label: string }[]
  /** The address of the page for a period, for the quick filters. */
  linkTo: (period: string) => string
  onApply: (period: string) => void
  /** Whether "All periods" can be chosen; a report always covers some period. */
  allowAll?: boolean
  today?: Date
}

/**
 * The period button of a report or list, after Moneybird: named after the
 * period shown, it drops down quick periods and the period picker.
 */
export function PeriodFilter({ period, title, quick, linkTo, onApply, allowAll = false, today: now }: Props) {
  const { t, i18n } = useTranslation()
  const today = useMemo(() => now ?? new Date(), [now])
  const range = period === allPeriods ? null : parsePeriod(period, today)
  const named = quick.find((q) => q.value === period)
  const label = named ? named.label : range ? describePeriod(range, i18n.language) : t('list.allPeriods')
  return (
    <FilterPanel label={label} title={title} quick={quick.map((q) => ({ label: q.label, to: linkTo(q.value), current: q.value === period }))}>
      <PeriodForm period={period} today={today} allowAll={allowAll} onApply={onApply} />
    </FilterPanel>
  )
}

function PeriodForm({ period, today, allowAll, onApply }: { period: string; today: Date; allowAll: boolean; onApply: (period: string) => void }) {
  const [range, setRange] = useState(() => (period === allPeriods ? null : parsePeriod(period, today)))
  return (
    <AdvancedFilter onApply={() => onApply(range ? periodValue(range, today) : allowAll ? allPeriods : period)}>
      <PeriodPicker value={range} today={today} allowAll={allowAll} onChange={setRange} />
    </AdvancedFilter>
  )
}
