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
import { fileName, reportSheet } from './exportFile'
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
  /** What a column covers; none for one column over the whole period, which only the balance sheet offers. */
  granularity?: Granularity | 'none'
  order: Order
  /** A report's own choice of how it shows, such as the VAT report's format. */
  format: string
  page: number
  today: Date
}

/** The granularity to ask the API for: none for a report in one column. */
export const apiGranularity = (f: ReportFilter): Granularity | undefined => (f.granularity === 'none' ? undefined : f.granularity)

const defaults: Record<FilterKind, string> = { granular: 'this_quarter', period: 'this_quarter', until: 'this_month', year: 'this_year' }

/** The report's filter from the address, Moneybird's defaults where it says nothing: this quarter per month. */
export function useReportFilter(kind: FilterKind): ReportFilter {
  const { administrationId = '' } = useParams()
  const [search] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const asked = parsePeriod(search.get('period') ?? '', today) ?? parsePeriod(defaults[kind], today)!
  const range = kind === 'until' ? lastMonth(asked) : kind === 'year' ? yearRange(yearOfMonth(asked.to)) : asked
  const g = search.get('granularity') as Granularity | 'none' | null
  const o = search.get('order') as Order | null
  return {
    administrationId,
    kind,
    range,
    period: periodValue(range, today),
    granularity: kind === 'granular' ? (g && (g === 'none' || granularities.includes(g)) ? g : 'month') : undefined,
    order: o && orders.includes(o) ? o : 'alphabetical',
    format: search.get('format') ?? '',
    page: Math.max(1, Number(search.get('page')) || 1),
    today,
  }
}

/** The address of the report with some of its filter changed; a new filter starts at the first page. */
function useFilterLink(f: ReportFilter) {
  const { pathname } = useLocation()
  return (change: { range?: MonthRange; granularity?: Granularity | 'none'; order?: Order; format?: string }) => {
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
    const format = change.format ?? f.format
    if (format) {
      params.set('format', format)
    }
    return `${pathname}?${params.toString()}`
  }
}

/** Downloads the report's tables on the page as an Excel file named after the report, as Moneybird's Export. */
async function exportExcel(title: string, locale: string) {
  const tables = Array.from(document.querySelectorAll<HTMLTableElement>('.content table'))
  const { default: writeXlsxFile } = await import('write-excel-file/browser')
  await writeXlsxFile(reportSheet(title, tables, locale), { sheet: title.replace(/[[\]:*?/\\]/g, ' ').slice(0, 31) }).toFile(fileName(title))
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
  /** Whether the columns are offered as Moneybird's balance sheet's "Compare periods", with No comparison. */
  comparable?: boolean
  /** The report's formats to choose from, such as the VAT report's, with the name of the choice. */
  formats?: { legend: string; options: { value: string; label: string }[] }
  /** Whether the filter button says what a column covers, as "per month"; Moneybird's aged receivables do not. */
  sayPer?: boolean
  /** Moneybird's introduction behind the More information button. */
  info?: Info
  /** Whether the report can be exported; Moneybird's overview cannot. */
  exportable?: boolean
  /** More buttons before Export, such as Show graphs. */
  actions?: ReactNode
  children: ReactNode
}

/**
 * The page every report shares, after Moneybird's: the filter button saying
 * what is shown with buttons to the period before and after it, More
 * information, and Export; the report in a card below.
 */
export function ReportPage({ filter, name, title, orderable = false, comparable = false, formats, sayPer = true, exportable = true, info, actions, children }: Props) {
  const { t, i18n } = useTranslation()
  const link = useFilterLink(filter)
  const infoId = useId()
  const [infoShown, setInfoShown] = useState(false)
  const what = rangeName(filter.range, t, i18n.language)
  const label = filter.kind === 'granular' && sayPer && filter.granularity !== 'none'
    ? t('reports.label.granular', { name, period: what, per: t(`reports.per.${filter.granularity}`) })
    : t(`reports.label.${filter.kind === 'granular' ? 'period' : filter.kind}`, { name, period: what })
  const lead = (
    <div className="report-filter">
      {filter.kind === 'year' ? (
        <span className="btn btn--secondary report-filter__static">{label}</span>
      ) : (
        <FilterPanel label={label} title={t('reports.filters')}>
          <ReportForm filter={filter} orderable={orderable} comparable={comparable} formats={formats} />
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
          {exportable && (
            <ActionMenu
              label={t('list.export')}
              items={[
                { label: t('reports.pdf'), onSelect: () => window.print() },
                { label: t('reports.excel'), onSelect: () => void exportExcel(label, i18n.language) },
              ]}
            />
          )}
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
function ReportForm({ filter, orderable, comparable, formats }: { filter: ReportFilter; orderable: boolean; comparable: boolean; formats?: Props['formats'] }) {
  const { t } = useTranslation()
  const [, setSearch] = useSearchParams()
  const link = useFilterLink(filter)
  const [range, setRange] = useState<MonthRange | null>(filter.range)
  const [granularity, setGranularity] = useState(filter.granularity)
  const [order, setOrder] = useState(filter.order)
  const [format, setFormat] = useState(filter.format || formats?.options[0]?.value || '')
  const apply = () => {
    const chosen = range ?? filter.range
    const to = link({ range: filter.kind === 'until' ? lastMonth(chosen) : chosen, granularity, order, format })
    setSearch(new URLSearchParams(to.slice(to.indexOf('?') + 1)))
  }
  return (
    <AdvancedFilter value={{ range, granularity, order, format }} onApply={apply}>
      <PeriodPicker value={range} today={filter.today} onChange={setRange} />
      {granularity && (
        <RadioFilter
          legend={t(comparable ? 'reports.comparePeriods' : 'reports.showPer')}
          value={granularity}
          onChange={(g) => setGranularity(g as Granularity | 'none')}
          options={[
            ...(comparable ? [{ value: 'none', label: t('reports.noComparison') }] : []),
            ...granularities.map((g) => ({ value: g, label: t(`reports.granularities.${g}`) })),
          ]}
        />
      )}
      {formats && <RadioFilter legend={formats.legend} value={format} onChange={setFormat} options={formats.options} />}
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
