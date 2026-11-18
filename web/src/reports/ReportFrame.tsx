import { type ReactNode, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom'
import { ChevronIcon, InfoIcon } from '../layout/icons'
import { Page } from '../layout/Page'
import { AdvancedFilter, FilterPanel, RadioFilter } from '../list/FilterPanel'
import { GettingStarted, type Step } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import { type MonthRange, parsePeriod, periodValue, yearOfMonth } from '../list/period'
import { PeriodPicker } from '../list/PeriodPicker'
import { type Granularity, granularities, lastMonth, rangeName, shiftRange, yearRange } from './period'
import './reports.css'

/**
 * How a report is filtered, after Moneybird's: on a period split into
 * columns per month, quarter or year; on a period; on the month it looks back
 * from; or on a year.
 */
export type FilterKind = 'granular' | 'period' | 'until' | 'year'
/** The order of the categories, after Moneybird's "Sort by". */
export type Order = 'alphabetical' | 'account_id' | 'rgs'
const orders: Order[] = ['alphabetical', 'account_id', 'rgs']

export type ReportFilter = {
  administrationId: string
  kind: FilterKind
  range: MonthRange
  /** The period as the API takes it. */
  period: string
  granularity?: Granularity
  order: Order
  page: number
  today: Date
}

const defaults: Record<FilterKind, string> = { granular: 'this_quarter', period: 'this_quarter', until: 'this_month', year: 'this_year' }

/** The report's filter from the address, Moneybird's defaults where it says nothing: this quarter per month. */
export function useReportFilter(kind: FilterKind): ReportFilter {
  const { administrationId = '' } = useParams()
  const [search] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const asked = parsePeriod(search.get('period') ?? '', today) ?? parsePeriod(defaults[kind], today)!
  const range = kind === 'until' ? lastMonth(asked) : kind === 'year' ? yearRange(yearOfMonth(asked.to)) : asked
  const g = search.get('granularity') as Granularity | null
  const o = search.get('order') as Order | null
  return {
    administrationId,
    kind,
    range,
    period: periodValue(range, today),
    granularity: kind === 'granular' ? (g && granularities.includes(g) ? g : 'month') : undefined,
    order: o && orders.includes(o) ? o : 'alphabetical',
    page: Math.max(1, Number(search.get('page')) || 1),
    today,
  }
}

/** The address of the report with some of its filter changed; a new filter starts at the first page. */
function useFilterLink(f: ReportFilter) {
  const { pathname } = useLocation()
  return (change: { range?: MonthRange; granularity?: Granularity; order?: Order }) => {
    const params = new URLSearchParams()
    params.set('period', periodValue(change.range ?? f.range, f.today))
    const granularity = change.granularity ?? f.granularity
    if (granularity) {
      params.set('granularity', granularity)
    }
    const order = change.order ?? f.order
    if (order !== 'alphabetical') {
      params.set('order', order)
    }
    return `${pathname}?${params.toString()}`
  }
}

export type Info = { title: string; text: string; steps: Step[] }

type Props = {
  filter: ReportFilter
  /** The report's name in its filter button, such as "Profit and loss statement". */
  name: string
  /** The page's title for screen readers, the report's name in the menu. */
  title: string
  /** Whether the categories can be sorted. */
  orderable?: boolean
  /** Whether the filter button says what a column covers, as "per month"; Moneybird's aged receivables do not. */
  sayPer?: boolean
  /** Moneybird's introduction behind the More information button. */
  info?: Info
  /** More buttons before Export, such as Show graphs. */
  actions?: ReactNode
  children: ReactNode
}

/**
 * The page every report shares, after Moneybird's: the filter button saying
 * what is shown with buttons to the period before and after it, More
 * information, and Export; the report in a card below.
 */
export function ReportPage({ filter, name, title, orderable = false, sayPer = true, info, actions, children }: Props) {
  const { t, i18n } = useTranslation()
  const link = useFilterLink(filter)
  const infoId = useId()
  const [infoShown, setInfoShown] = useState(false)
  const what = rangeName(filter.range, t, i18n.language)
  const label = filter.kind === 'granular' && sayPer
    ? t('reports.label.granular', { name, period: what, per: t(`reports.per.${filter.granularity}`) })
    : t(`reports.label.${filter.kind === 'granular' ? 'period' : filter.kind}`, { name, period: what })
  const lead = (
    <div className="report-filter">
      {filter.kind === 'year' ? (
        <span className="btn btn--secondary report-filter__static">{label}</span>
      ) : (
        <FilterPanel label={label} title={t('reports.filters')}>
          <ReportForm filter={filter} orderable={orderable} />
        </FilterPanel>
      )}
      {filter.kind !== 'until' && <Steps filter={filter} link={link} />}
    </div>
  )
  return (
    <Page
      wide
      title={title}
      lead={lead}
      actions={
        <>
          {info && (
            <button
              type="button"
              className={`btn btn--secondary btn--icon${infoShown ? ' is-active' : ''}`}
              aria-label={t('list.moreInfo')}
              title={t('list.moreInfo')}
              aria-expanded={infoShown}
              aria-controls={infoId}
              onClick={() => setInfoShown(!infoShown)}
            >
              <InfoIcon />
            </button>
          )}
          {actions}
          <ActionMenu
            label={t('list.export')}
            items={[{ label: t('reports.pdf'), unavailable: true }, { label: t('reports.excel'), unavailable: true }]}
          />
        </>
      }
    >
      {info && infoShown && <GettingStarted id={infoId} title={info.title} text={info.text} steps={info.steps} />}
      {children}
    </Page>
  )
}

/** The buttons to the period before and after the one shown, named after where they go. */
function Steps({ filter, link }: { filter: ReportFilter; link: ReturnType<typeof useFilterLink> }) {
  const { t, i18n } = useTranslation()
  const step = (by: number) => {
    const range = shiftRange(filter.range, by)
    const name = t('reports.goTo', { period: rangeName(range, t, i18n.language) })
    return (
      <Link className="btn btn--secondary btn--icon" to={link({ range })} aria-label={name} title={name}>
        <ChevronIcon className={by < 0 ? 'report-filter__back' : undefined} />
      </Link>
    )
  }
  return (
    <div className="button-group">
      {step(-1)}
      {step(1)}
    </div>
  )
}

/** The filter panel: the period picker, and for a report in columns what a column covers, and the order of the categories. */
function ReportForm({ filter, orderable }: { filter: ReportFilter; orderable: boolean }) {
  const { t } = useTranslation()
  const [, setSearch] = useSearchParams()
  const link = useFilterLink(filter)
  const [range, setRange] = useState<MonthRange | null>(filter.range)
  const [granularity, setGranularity] = useState(filter.granularity)
  const [order, setOrder] = useState(filter.order)
  const apply = () => {
    const chosen = range ?? filter.range
    const to = link({ range: filter.kind === 'until' ? lastMonth(chosen) : chosen, granularity, order })
    setSearch(new URLSearchParams(to.slice(to.indexOf('?') + 1)))
  }
  return (
    <AdvancedFilter value={{ range, granularity, order }} onApply={apply}>
      <PeriodPicker value={range} today={filter.today} onChange={setRange} />
      {granularity && (
        <RadioFilter
          legend={t('reports.showPer')}
          value={granularity}
          onChange={(g) => setGranularity(g as Granularity)}
          options={granularities.map((g) => ({ value: g, label: t(`reports.granularities.${g}`) }))}
        />
      )}
      {orderable && (
        <RadioFilter
          legend={t('reports.sortBy')}
          value={order}
          onChange={(o) => setOrder(o as Order)}
          options={orders.map((o) => ({ value: o, label: t(`reports.orders.${o}`) }))}
        />
      )}
    </AdvancedFilter>
  )
}
