import { Fragment, type ReactNode, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { useExternalInvoice } from '../external/queries'
import { incomingTitle, useTypedDocument } from '../incoming/queries'
import { invoiceTitle, useSalesInvoice } from '../invoices/queries'
import { type AccountType, balanceSheetTypes, type LedgerAccount, profitAndLossTypes, useLedgerAccounts } from '../bookkeeping/queries'
import { contactName, useContact } from '../contacts/queries'
import { CaretIcon, InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Pagination } from '../layout/Pagination'
import { useProject } from '../projects/queries'
import { BoxTable } from '../vat/BoxTable'
import { IcpTable } from '../vat/IcpTable'
import '../vat/vat.css'
import { type MonthRange, parsePeriod } from '../list/period'
import { BarsChart, Legend } from './charts'
import { lastDay, monthsParam, partName, rangeName } from './period'
import {
  type AgingDocument, type AgingKind, amounts, balances, type ContactReportKind, type ProjectReportKind, reportPerPage, type Subject, type SubjectReport,
  useAgingDocuments, useAgingReport, useAssetsReport, useBalanceSheet, useCashFlow, useContactReport, useGeneralLedger, useProfitLoss, useProjectReport, useTaxReport,
  useVatOverview, type VatFormat, vatFormats,
} from './queries'
import { apiGranularity, type Info, type ReportFilter, ReportPage, useReportFilter } from './ReportFrame'
import {
  Amounts, CategoryRows, categoryTree, CollapseAll, CollapseProvider, type Column, PercentProvider, ReportCard, ReportTable, Section, TotalRow, type TreeNode, useAmount,
  useCollapse,
} from './ReportTable'

/** The parts of the period a report came back with, or the whole period when it is not split. */
function partsOf(f: ReportFilter, parts: { period?: string }[] | undefined): string[] {
  const list = (parts ?? []).map((p) => p.period ?? '').filter(Boolean)
  return list.length > 0 ? list : [monthsParam(f.range)]
}

function useColumns(parts: string[]): Column[] {
  const { t, i18n } = useTranslation()
  return parts.map((p) => ({ key: p, label: partName(p, t, i18n.language), period: p }))
}

function useAccounts(administrationId: string) {
  return useLedgerAccounts(administrationId).data
}

/** Where a category's bookings in a period are listed. */
function ledgerLink(f: ReportFilter, columns: Column[]) {
  return (id: string, column?: number) => {
    if (id.startsWith('year:')) {
      return undefined
    }
    const period = column === undefined ? f.period : columns[column]?.period
    return `/${f.administrationId}/report/journal_entries?ledger_account_id=${id}&period=${period ?? f.period}`
  }
}

/** Amounts per category for each part, from a list of amounts by category per part. */
function perPart(maps: Map<string, number>[]): Map<string, number[]> {
  const out = new Map<string, number[]>()
  maps.forEach((m, i) => {
    for (const [id, v] of m) {
      const row = out.get(id) ?? Array<number>(maps.length).fill(0)
      row[i] = v
      out.set(id, row)
    }
  })
  return out
}

function useTree(values: Map<string, number[]>, accounts: LedgerAccount[] | undefined, width: number, f: ReportFilter) {
  const { t } = useTranslation()
  return useMemo(
    () => categoryTree(values, accounts ?? [], width, f.order, (name) => t('reports.general', { name })),
    [values, accounts, width, f.order, t],
  )
}

const sum = (nodes: TreeNode[], width: number) => nodes.reduce((s, n) => s.map((v, i) => v + (n.values[i] ?? 0)), Array<number>(width).fill(0))

function Empty({ f }: { f: ReportFilter }) {
  const { t, i18n } = useTranslation()
  return (
    <section className="report-card report-empty">
      <h2>{t('reports.noInformation', { period: rangeName(f.range, t, i18n.language) })}</h2>
    </section>
  )
}

function Failed() {
  const { t } = useTranslation()
  return <p className="empty">{t('journal.loadError')}</p>
}

function useCurrency(administrationId: string) {
  return useAdministration(administrationId).data?.currency ?? 'EUR'
}

/** Moneybird's introductions to its reports, behind More information. */
function useInfo(key: 'profitLoss' | 'balanceSheet' | 'vat'): Info {
  const { t } = useTranslation()
  const icons = { profitLoss: ['coins', 'file', 'chart'], balanceSheet: ['bank', 'scale', 'check'], vat: ['file', 'calendar', 'coins'] } as const
  return {
    title: t(`reports.info.${key}.title`),
    text: t(`reports.info.${key}.text`),
    steps: [0, 1, 2].map((i) => ({ icon: icons[key][i] ?? 'file', title: t(`reports.info.${key}.steps.${i}.title`), text: t(`reports.info.${key}.steps.${i}.text`) })),
  }
}

/** The profit and loss categories per month, quarter or year of the period, and the profits. */
export function ProfitLossPage() {
  const { t } = useTranslation()
  const f = useReportFilter('granular')
  const report = useProfitLoss(f.administrationId, { period: f.period, granularity: apiGranularity(f) })
  const accounts = useAccounts(f.administrationId)
  const currency = useCurrency(f.administrationId)
  const info = useInfo('profitLoss')
  const amount = useAmount()
  const r = report.data
  const parts = useMemo(() => (r?.periods?.length ? r.periods : r ? [r] : []), [r])
  const columns = useColumns(partsOf(f, r?.periods))
  const width = columns.length
  const of = (key: 'revenue_by_ledger_account' | 'direct_costs_by_ledger_account' | 'expenses_by_ledger_account' | 'other_income_expenses_by_ledger_account') =>
    perPart(parts.map((p) => amounts(p[key])))
  const revenue = useTree(useMemo(() => of('revenue_by_ledger_account'), [parts]), accounts, width, f)
  const direct = useTree(useMemo(() => of('direct_costs_by_ledger_account'), [parts]), accounts, width, f)
  const expenses = useTree(useMemo(() => of('expenses_by_ledger_account'), [parts]), accounts, width, f)
  const other = useTree(useMemo(() => of('other_income_expenses_by_ledger_account'), [parts]), accounts, width, f)
  const all = useMemo(() => [...revenue, ...direct, ...expenses, ...other], [revenue, direct, expenses, other])
  const collapse = useCollapse(all)
  const [percent, setPercent] = useState(false)
  const [graphs, setGraphs] = useState(true)
  const title = t('sidenav.items.profitLoss')
  const graphsButton = (
    <button
      type="button"
      className={`btn btn--secondary btn--icon${graphs ? ' is-active' : ''}`}
      aria-label={t('reports.showGraphs')}
      title={t('reports.showGraphs')}
      aria-pressed={graphs}
      onClick={() => setGraphs(!graphs)}
    >
      <ListIcon kind="chart" />
    </button>
  )
  const page = (children: ReactNode) => <ReportPage filter={f} name={title} title={title} orderable info={info} actions={graphsButton}>{children}</ReportPage>
  if (!r || !accounts) {
    return page(report.error ? <Failed /> : <Loading />)
  }
  const totals = (key: 'total_revenue' | 'gross_profit' | 'total_expenses' | 'net_profit' | 'operating_profit') => parts.map((p) => Number(p[key] ?? 0))
  const link = ledgerLink(f, columns)
  const span = width * (percent ? 2 : 1) + 1
  const tools = (
    <>
      <button type="button" className="report-tool" aria-pressed={percent} onClick={() => setPercent(!percent)}>
        <span className={`report-tool__switch${percent ? ' is-on' : ''}`} aria-hidden="true" />
        <span>{t('reports.showPercentages')}</span>
      </button>
      <CollapseAll collapse={collapse} />
    </>
  )
  const labels = columns.map((c) => String(c.label))
  const revenueValues = totals('total_revenue')
  const costs = revenueValues.map((v, i) => v - (totals('net_profit')[i] ?? 0))
  return page(
    <>
      {graphs && (
        <div className="report-graphs">
          <section className="report-graph">
            <h2>{t('reports.netResult')}</h2>
            <BarsChart label={t('reports.netResult')} categories={labels} series={[{ key: 'net', label: t('reports.netProfit'), values: totals('net_profit'), color: 1 }]} />
          </section>
          <section className="report-graph">
            <h2>{t('reports.costAndRevenue')}</h2>
            <BarsChart
              label={t('reports.costAndRevenue')}
              categories={labels}
              series={[
                { key: 'revenue', label: t('reports.overview.revenue'), values: revenueValues, color: 1 },
                { key: 'costs', label: t('reports.costs'), values: costs, color: 2 },
              ]}
            />
            <Legend items={[
              { key: 'revenue', label: t('reports.overview.revenue'), value: amount(revenueValues.reduce((a, b) => a + b, 0)), color: 1 },
              { key: 'costs', label: t('reports.costs'), value: amount(costs.reduce((a, b) => a + b, 0)), color: 2 },
            ]} />
          </section>
        </div>
      )}
      <ReportCard note={t('reports.excludingVat', { currency })}>
        <CollapseProvider value={collapse.context}>
          <PercentProvider revenue={percent ? totals('total_revenue') : null}>
            <ReportTable label={title} columns={columns} tools={tools}>
              <Section title={t('bookkeeping.accountTypes.revenue')} span={span}>
                <CategoryRows nodes={revenue} link={link} />
                <TotalRow label={t('reports.totalRevenue')} values={totals('total_revenue')} />
              </Section>
              <Section title={t('bookkeeping.accountTypes.direct_costs')} span={span}>
                <CategoryRows nodes={direct} link={link} />
                <TotalRow label={t('reports.totalDirectCosts')} values={sum(direct, width)} />
                <TotalRow label={t('reports.grossProfit')} values={totals('gross_profit')} />
              </Section>
              <Section title={t('bookkeeping.accountTypes.expenses')} span={span}>
                <CategoryRows nodes={expenses} link={link} />
                <TotalRow label={t('reports.totalExpenses')} values={sum(expenses, width)} />
              </Section>
              {other.length > 0 && (
                <Section title={t('bookkeeping.accountTypes.other_income_expenses')} span={span}>
                  <TotalRow label={t('reports.operatingProfit')} values={totals('operating_profit')} />
                  <CategoryRows nodes={other} link={link} />
                  <TotalRow label={t('reports.totalOther')} values={sum(other, width)} />
                </Section>
              )}
              <Section span={span}>
                <TotalRow label={t('reports.netProfit')} values={totals('net_profit')} grand />
              </Section>
            </ReportTable>
          </PercentProvider>
        </CollapseProvider>
      </ReportCard>
    </>,
  )
}

type Side = { types: AccountType[]; label: string }

/** The balance sheet at the start of the period and the end of each of its parts: assets on the debit side, equity and liabilities on the credit side. */
export function BalanceSheetPage() {
  const { t } = useTranslation()
  const f = useReportFilter('granular')
  const report = useBalanceSheet(f.administrationId, { period: f.period, granularity: apiGranularity(f) })
  const before: MonthRange = { from: f.range.from - 1, to: f.range.from - 1 }
  const opening = useBalanceSheet(f.administrationId, { period: monthsParam(before) })
  const accounts = useAccounts(f.administrationId)
  const info = useInfo('balanceSheet')
  const title = t('sidenav.items.balanceSheet')
  const r = report.data
  // Without comparison, only the end of the period; with it, the start and the end of each part, as Moneybird.
  const single = f.granularity === 'none'
  const sheets = useMemo(() => {
    if (!r || !opening.data) {
      return []
    }
    return single ? [r] : [opening.data, ...(r.periods?.length ? r.periods : [r])]
  }, [r, opening.data, single])
  const ends = useMemo(() => {
    if (single) {
      return [lastDay(f.range.to)]
    }
    const parts = partsOf(f, r?.periods).map((p) => parsePeriod(p, f.today)?.to ?? f.range.to)
    return [lastDay(f.range.from - 1), ...parts.map(lastDay)]
  }, [f, r?.periods, single])
  const columns: Column[] = ends.map((d) => ({ key: d, label: d }))
  const width = columns.length
  const sides: Side[] = [
    { types: ['non_current_assets', 'current_assets'], label: t('reports.debitSide') },
    { types: ['equity', 'provisions', 'non_current_liabilities', 'current_liabilities'], label: t('reports.creditSide') },
  ]
  const values = useMemo(() => {
    const out: Partial<Record<AccountType, Map<string, number[]>>> = {}
    for (const type of balanceSheetTypes) {
      out[type] = perPart(sheets.map((s) => balances((type === 'non_current_assets' || type === 'current_assets'
        ? s.debit?.[type]
        : s.credit?.[type as 'equity' | 'provisions' | 'non_current_liabilities' | 'current_liabilities']))))
    }
    return out
  }, [sheets])
  const trees = useMemo(() => {
    const out: Partial<Record<AccountType, TreeNode[]>> = {}
    for (const type of balanceSheetTypes) {
      out[type] = categoryTree(values[type] ?? new Map(), accounts ?? [], width, f.order, (name) => t('reports.general', { name }))
    }
    const years = [...new Set(sheets.flatMap((s) => (s.credit?.open_years ?? []).map((y) => y.year ?? '')))].sort().reverse()
    if (years.length > 0) {
      const children = years.map((y) => ({
        key: `year:${y}`, id: `year:${y}`, name: y, children: [],
        values: sheets.map((s) => Number(s.credit?.open_years?.find((o) => o.year === y)?.value ?? 0)),
      }))
      out.equity = [...(out.equity ?? []), { key: 'year:open', id: 'year:open', name: t('reports.openYears'), children, values: sum(children, width) }]
    }
    return out
  }, [values, accounts, width, f.order, t, sheets])
  const collapse = useCollapse(useMemo(() => balanceSheetTypes.flatMap((type) => trees[type] ?? []), [trees]))
  const page = (children: ReactNode) => <ReportPage filter={f} name={title} title={title} orderable comparable info={info}>{children}</ReportPage>
  if (!r || !opening.data || !accounts) {
    return page(report.error || opening.error ? <Failed /> : <Loading />)
  }
  const link = ledgerLink(f, [])
  const totals = (side: 'debit' | 'credit') => sheets.map((s) => Number(s[side]?.total ?? 0))
  return page(
    <ReportCard>
      <CollapseProvider value={collapse.context}>
        <div className="report-sides">
          {sides.map((side, i) => (
            <div key={side.label} className="report-side">
              <p className="report-side__label" aria-hidden="true">{side.label}</p>
              <ReportTable label={side.label} columns={columns} tools={i === 0 ? <CollapseAll collapse={collapse} /> : undefined}>
                {side.types.filter((type) => (trees[type] ?? []).length > 0).map((type) => (
                  <Section key={type} title={t(`bookkeeping.accountTypes.${type}`)} span={width + 1}>
                    <CategoryRows nodes={trees[type] ?? []} link={(id, column) => (column === undefined ? link(id) : undefined)} />
                  </Section>
                ))}
                <Section span={width + 1}>
                  <TotalRow label={<span className="sr-only">{t('reports.totalOf', { side: side.label })}</span>} values={totals(i === 0 ? 'debit' : 'credit')} grand />
                </Section>
              </ReportTable>
            </div>
          ))}
        </div>
      </CollapseProvider>
    </ReportCard>,
  )
}

/**
 * Moneybird's VAT report of a period in one of three formats: the boxes of
 * the Dutch return, the Dutch ICP declaration, or what was charged on and
 * the VAT per rate.
 */
export function TaxReportPage() {
  const { t, i18n } = useTranslation()
  const f = useReportFilter('period')
  const format: VatFormat = vatFormats.includes(f.format as VatFormat) ? (f.format as VatFormat) : 'dutch'
  const days = vatDays(f.range)
  const overview = useVatOverview(f.administrationId, format === 'dutch_icp' ? 'icp_declaration' : 'vat_return', days, format !== 'overview')
  const rates = useTaxReport(f.administrationId, { period: f.period })
  const info = useInfo('vat')
  const title = t('sidenav.items.vatSummary')
  const name = t(`reports.vatNames.${format}`)
  const page = (children: ReactNode) => (
    <ReportPage
      filter={f}
      name={name}
      title={title}
      info={info}
      formats={{ legend: t('reports.vatFormat'), options: vatFormats.map((v) => ({ value: v, label: t(`reports.vatFormats.${v}`) })) }}
    >
      {children}
    </ReportPage>
  )
  if (format !== 'overview') {
    if (!overview.data) {
      return page(overview.error ? <Failed /> : <Loading />)
    }
    if (format === 'dutch_icp') {
      return page(overview.data.icp_lines.length === 0
        ? <Empty f={f} />
        : <ReportCard><IcpTable lines={overview.data.icp_lines} label={name} /></ReportCard>)
    }
    return page(<ReportCard><BoxTable figures={overview.data} periodName={rangeName(f.range, t, i18n.language)} label={name} /></ReportCard>)
  }
  if (!rates.data) {
    return page(rates.error ? <Failed /> : <Loading />)
  }
  const list = rates.data.tax_rates ?? []
  if (list.length === 0) {
    return page(<Empty f={f} />)
  }
  return page(
    <ReportCard>
      <ReportTable label={name} columns={[{ key: 'turnover', label: t('reports.turnover') }, { key: 'vat', label: t('reports.salesTax') }]} tools={t('reports.taxRate')}>
        <Section span={3}>
          {list.map((r) => (
            <tr key={`${r.tax_rate_id}`} className="report-table__category">
              <th scope="row">
                <span className="report-table__name">
                  <span>{r.name}</span>{' '}
                  <span className="report-table__code">{r.report_reference} · {t(`reports.rateTypes.${r.type === 'sales_invoice' ? 'sales' : 'purchases'}`)}</span>
                </span>
              </th>
              <Amounts values={[Number(r.amount ?? 0), Number(r.tax ?? 0)]} />
            </tr>
          ))}
        </Section>
      </ReportTable>
    </ReportCard>,
  )
}

/** The first and the last day of the months, as the VAT overview takes them: 20261001..20261231. */
function vatDays(range: MonthRange): string {
  const first = lastDay(range.from - 1)
  const start = new Date(`${first}T00:00:00Z`)
  start.setUTCDate(start.getUTCDate() + 1)
  return `${start.toISOString().slice(0, 10).replaceAll('-', '')}..${lastDay(range.to).replaceAll('-', '')}`
}

/** The cash received and paid per part of the period, between the opening and the closing balance. */
export function CashFlowPage() {
  const { t } = useTranslation()
  const f = useReportFilter('granular')
  const report = useCashFlow(f.administrationId, { period: f.period, granularity: apiGranularity(f) })
  const accounts = useAccounts(f.administrationId)
  const title = t('sidenav.items.cashFlow')
  const r = report.data
  const parts = useMemo(() => (r?.periods?.length ? r.periods : r ? [r] : []), [r])
  const columns = useColumns(partsOf(f, r?.periods))
  const width = columns.length
  const received = useTree(useMemo(() => perPart(parts.map((p) => amounts(p.cash_received_by_ledger_account))), [parts]), accounts, width, f)
  const paid = useTree(useMemo(() => perPart(parts.map((p) => amounts(p.cash_paid_by_ledger_account))), [parts]), accounts, width, f)
  const collapse = useCollapse(useMemo(() => [...received, ...paid], [received, paid]))
  const page = (children: ReactNode) => <ReportPage filter={f} name={title} title={title}>{children}</ReportPage>
  if (!r || !accounts) {
    return page(report.error ? <Failed /> : <Loading />)
  }
  const moved = received.length + paid.length > 0 || parts.some((p) => Number(p.opening_balance ?? 0) !== 0)
  if (!moved) {
    return page(<Empty f={f} />)
  }
  const link = ledgerLink(f, columns)
  const span = width + 1
  return page(
    <ReportCard>
      <CollapseProvider value={collapse.context}>
        <ReportTable label={title} columns={columns} tools={<CollapseAll collapse={collapse} />}>
          <Section span={span}>
            <TotalRow label={t('reports.openingBalance')} values={parts.map((p) => Number(p.opening_balance ?? 0))} />
          </Section>
          <Section title={t('reports.received')} span={span}>
            <CategoryRows nodes={received} link={link} />
            <TotalRow label={t('reports.totalReceived')} values={sum(received, width)} />
          </Section>
          <Section title={t('reports.paid')} span={span}>
            <CategoryRows nodes={paid} link={link} />
            <TotalRow label={t('reports.totalPaid')} values={sum(paid, width)} />
          </Section>
          <Section span={span}>
            <TotalRow label={t('reports.closingBalance')} values={parts.map((p) => Number(p.closing_balance ?? 0))} grand />
          </Section>
        </ReportTable>
      </CollapseProvider>
    </ReportCard>,
  )
}

function ContactName({ administrationId, id, link = true, to }: { administrationId: string; id: string | null; link?: boolean; to?: string }) {
  const { t } = useTranslation()
  const contact = useContact(administrationId, id ?? undefined)
  if (id == null) {
    return <>{t('reports.noContact')}</>
  }
  const name = contact.data ? contactName(contact.data) : id
  return link ? <Link to={to ?? `/${administrationId}/contacts/${id}`}>{name}</Link> : <>{name}</>
}

function ProjectName({ administrationId, id }: { administrationId: string; id: string | null }) {
  const { t } = useTranslation()
  const project = useProject(administrationId, id ?? undefined)
  if (id == null) {
    return <>{t('reports.noProject')}</>
  }
  return <Link to={`/${administrationId}/projects/${id}`}>{project.data?.name ?? id}</Link>
}

/** A report per contact or project: a row for each, a column per part of the period, and the page's total. */
function SubjectTable({ f, report, label, totalLabel, name }: { f: ReportFilter; report: SubjectReport; label: string; totalLabel: string; name: (id: string | null) => ReactNode }) {
  const parts = report.parts.length > 0 ? report.parts : [{ period: monthsParam(f.range), rows: report.rows }]
  const columns = useColumns(parts.map((p) => p.period))
  const value = (rows: Subject[], id: string | null) => Number(rows.find((r) => r.id === id)?.amount ?? 0)
  const span = columns.length + 1
  return (
    <ReportCard>
      <ReportTable label={label} columns={columns}>
        <Section span={span}>
          {report.rows.map((row) => (
            <tr key={String(row.id)} className="report-table__category">
              <th scope="row"><span className="report-table__name">{name(row.id)}</span></th>
              <Amounts values={parts.map((p) => value(p.rows, row.id))} />
            </tr>
          ))}
        </Section>
        <Section span={span}>
          <TotalRow label={totalLabel} values={parts.map((p) => report.rows.reduce((s, row) => s + value(p.rows, row.id), 0))} grand />
        </Section>
      </ReportTable>
      <Pagination total={report.total} perPage={reportPerPage} />
    </ReportCard>
  )
}

const contactReports: Record<ContactReportKind, { menu: string; name: string; total: string }> = {
  debtors: { menu: 'debtors', name: 'agedReceivables', total: 'totalThisPage' },
  creditors: { menu: 'creditors', name: 'agedPayables', total: 'totalThisPage' },
  revenue_by_contact: { menu: 'revenueByContact', name: 'revenueByContact', total: 'total' },
  expenses_by_contact: { menu: 'expensesByContact', name: 'expensesByContact', total: 'total' },
}

/** An amount per contact per part of the period: the balances of debtors or creditors, or the revenue or costs per contact. */
export function ContactReportPage({ kind }: { kind: ContactReportKind }) {
  const { t } = useTranslation()
  // Revenue and costs per contact open one contact's own report from its row, as Moneybird's /report/revenue_by_contact/:id.
  const { contactId } = useParams()
  const { search } = useLocation()
  const names = contactReports[kind]
  const title = t(`sidenav.items.${names.menu}`)
  const f = useReportFilter('granular')
  const report = useContactReport(f.administrationId, kind, { period: f.period, granularity: apiGranularity(f) }, f.page, contactId)
  const contact = useContact(f.administrationId, contactId)
  const perContact = names.total === 'total'
  const list = `/${f.administrationId}/report/${kind}`
  const reportName = t(`reports.names.${names.name}`)
  const name = contactId ? t('reports.ofContact', { name: reportName, contact: contact.data ? contactName(contact.data) : '' }) : reportName
  const back = contactId ? <Link className="btn btn--secondary" to={`${list}${search}`}>{t('reports.backTo', { name: title })}</Link> : undefined
  const page = (children: ReactNode) => <ReportPage filter={f} name={name} title={title} sayPer={perContact} actions={back}>{children}</ReportPage>
  if (!report.data) {
    return page(report.error ? <Failed /> : <Loading />)
  }
  if (report.data.rows.length === 0) {
    return page(<Empty f={f} />)
  }
  return page(
    <SubjectTable
      f={f}
      report={report.data}
      label={title}
      totalLabel={t(`reports.${names.total}`)}
      name={(id) =>
        perContact && id != null && !contactId
          ? <ContactName administrationId={f.administrationId} id={id} to={`${list}/${id}${search}`} />
          : <ContactName administrationId={f.administrationId} id={id} />}
    />,
  )
}

/** The revenue or the costs booked per project per part of the period. */
export function ProjectReportPage({ kind }: { kind: ProjectReportKind }) {
  const { t } = useTranslation()
  const title = t(kind === 'revenue_by_project' ? 'sidenav.items.revenueByProject' : 'sidenav.items.expensesByProject')
  const f = useReportFilter('granular')
  const report = useProjectReport(f.administrationId, kind, { period: f.period, granularity: apiGranularity(f) }, f.page)
  const page = (children: ReactNode) => <ReportPage filter={f} name={title} title={title}>{children}</ReportPage>
  if (!report.data) {
    return page(report.error ? <Failed /> : <Loading />)
  }
  if (report.data.rows.length === 0) {
    return page(<Empty f={f} />)
  }
  return page(
    <SubjectTable f={f} report={report.data} label={title} totalLabel={t('reports.total')} name={(id) => <ProjectName administrationId={f.administrationId} id={id} />} />,
  )
}

/** What each contact had open at the end of a month, by age. */
export function AgingReportPage({ kind }: { kind: AgingKind }) {
  const { t } = useTranslation()
  const debtors = kind === 'debtors_aging'
  const title = t(debtors ? 'sidenav.items.debtorsAging' : 'sidenav.items.creditorsAging')
  const f = useReportFilter('until')
  const report = useAgingReport(f.administrationId, kind, f.period, f.page)
  const page = (children: ReactNode) => <ReportPage filter={f} name={t(debtors ? 'reports.names.receivablesAging' : 'reports.names.payablesAging')} title={title}>{children}</ReportPage>
  if (!report.data) {
    return page(report.error ? <Failed /> : <Loading />)
  }
  const items = report.data.items
  if (items.length === 0) {
    return page(<Empty f={f} />)
  }
  const names = items[0]?.aging_buckets?.map((b) => b.name ?? '') ?? []
  const columns: Column[] = names.map((n, i) => ({ key: `${i}`, label: n }))
  const row = (x: (typeof items)[number]) => names.map((_, i) => Number(x.aging_buckets?.[i]?.amount ?? 0))
  const span = columns.length + 1
  return page(
    <ReportCard>
      <ReportTable label={title} columns={columns}>
        <Section span={span}>
          {items.map((x) => (
            <AgingRow key={String(x.contact_id)} f={f} kind={kind} contactId={x.contact_id == null ? null : String(x.contact_id)} values={row(x)} />
          ))}
        </Section>
        <Section span={span}>
          <TotalRow label={t('reports.totalThisPage')} values={names.map((_, i) => items.reduce((s, x) => s + (row(x)[i] ?? 0), 0))} grand />
        </Section>
      </ReportTable>
      <Pagination total={report.data.total} perPage={reportPerPage} />
    </ReportCard>,
  )
}

/** A contact's row of the aging report, closed at first as at Moneybird; opened, it lists the invoices behind it. */
function AgingRow({ f, kind, contactId, values }: { f: ReportFilter; kind: AgingKind; contactId: string | null; values: number[] }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const contact = useContact(f.administrationId, contactId ?? undefined)
  const name = contactId == null ? t('reports.noContact') : contact.data ? contactName(contact.data) : contactId
  const docs = useAgingDocuments(f.administrationId, kind, f.period, contactId, open)
  return (
    <>
      <tr className="report-table__category is-depth-0">
        <th scope="row">
          <span className="report-table__name">
            <button
              type="button"
              className="report-table__fold"
              aria-expanded={open}
              aria-label={t(open ? 'reports.collapse' : 'reports.expand', { name })}
              onClick={() => setOpen(!open)}
            >
              <CaretIcon />
            </button>
            <span>{name}</span>
          </span>
        </th>
        <Amounts values={values} />
      </tr>
      {open && !docs.data && (
        <tr className="report-table__category is-depth-1"><td colSpan={values.length + 1}>{docs.error ? <Failed /> : <Loading />}</td></tr>
      )}
      {open && docs.data?.map((d) => (
        <tr key={`${d.document_type}-${d.document_id}`} className="report-table__category is-depth-1">
          <th scope="row"><span className="report-table__name"><AgingDocumentName administrationId={f.administrationId} document={d} /></span></th>
          <Amounts values={d.aging_buckets.map((b) => Number(b.amount))} />
        </tr>
      ))}
    </>
  )
}

/** Where each document of the aging report has its page. */
const agingPages: Record<AgingDocument['document_type'], string> = {
  SalesInvoice: 'sales_invoices', ExternalSalesInvoice: 'external_sales_invoices', PurchaseInvoice: 'documents', Receipt: 'documents',
}

/** An invoice or receipt by its number or reference, linked to its page. */
function AgingDocumentName({ administrationId, document: d }: { administrationId: string; document: AgingDocument }) {
  const { t } = useTranslation()
  const id = d.document_id
  const invoice = useSalesInvoice(administrationId, d.document_type === 'SalesInvoice' ? id : undefined)
  const external = useExternalInvoice(administrationId, d.document_type === 'ExternalSalesInvoice' ? id : undefined)
  const incoming = useTypedDocument(administrationId, d.document_type === 'PurchaseInvoice' || d.document_type === 'Receipt' ? d.document_type : undefined, id)
  const name = invoice.data ? invoiceTitle(invoice.data, t)
    : external.data ? (external.data.reference ?? '')
    : incoming.data && (incoming.data.type === 'PurchaseInvoice' || incoming.data.type === 'Receipt')
      ? incomingTitle(incoming.data.doc, incoming.data.type === 'Receipt' ? 'receipts' : 'purchase_invoices', t)
      : ''
  return <Link to={`/${administrationId}/${agingPages[d.document_type]}/${id}`}>{name || d.date}</Link>
}

/** A lifespan in seconds in words, such as "6 years" or "30 months". */
function useLifespan() {
  const { t } = useTranslation()
  return (seconds: string | null) => {
    if (!seconds) {
      return ''
    }
    const months = Math.round(Number(seconds) / (86400 * 30.436875))
    return months % 12 === 0 ? t('reports.years', { count: months / 12 }) : t('reports.months', { count: months })
  }
}

/** Each fixed asset in the year under its category: its purchase, residual and book value, and what it was depreciated. */
export function AssetsReportPage() {
  const { t } = useTranslation()
  const title = t('sidenav.items.assetsReport')
  const f = useReportFilter('year')
  const report = useAssetsReport(f.administrationId, f.period)
  const accounts = useAccounts(f.administrationId)
  const amount = useAmount()
  const lifespan = useLifespan()
  const page = (children: ReactNode) => <ReportPage filter={f} name={t('reports.names.assets')} title={title}>{children}</ReportPage>
  if (!report.data || !accounts) {
    return page(report.error ? <Failed /> : <Loading />)
  }
  if (report.data.length === 0) {
    return page(<Empty f={f} />)
  }
  const keys = ['purchase_value', 'calculated_purchase_value', 'residual_value', 'total_value_changes_since_purchase', 'value_at_end'] as const
  const byAccount = new Map<string, number[]>()
  for (const a of report.data) {
    const row = byAccount.get(String(a.ledger_account_id)) ?? keys.map(() => 0)
    byAccount.set(String(a.ledger_account_id), row.map((v, i) => v + Number(a[keys[i] as (typeof keys)[number]])))
  }
  const tree = categoryTree(byAccount, accounts, keys.length, 'alphabetical', (name) => name)
  const accountType = (id: string) => accounts.find((a) => a.id === id)?.account_type ?? 'non_current_assets'
  const types = [...new Set(tree.map((n) => accountType(n.id)))]
  const span = keys.length + 3
  const rows = (nodes: TreeNode[], depth: number): ReactNode => nodes.map((n) => (
    <Fragment key={n.key}>
      <tr className={`report-table__category is-depth-${Math.min(depth, 3)}`}>
        <th scope="row"><span className="report-table__name"><span>{n.name}</span></span></th>
        <td /><td />
        {n.values.map((v, i) => <td key={i} className="report-table__amount">{amount(v)}</td>)}
      </tr>
      {rows(n.children, depth + 1)}
      {n.children.length === 0 && report.data.filter((a) => String(a.ledger_account_id) === n.id).map((a) => (
        <tr key={String(a.asset_id)} className={`report-table__category is-depth-${Math.min(depth + 1, 3)}`}>
          <th scope="row"><span className="report-table__name"><Link to={`/${f.administrationId}/assets/${a.asset_id}`}>{a.name}</Link></span></th>
          <td>{a.purchase_date}</td>
          <td>{lifespan(a.lifespan)}</td>
          {keys.map((k) => <td key={k} className="report-table__amount">{amount(a[k])}</td>)}
        </tr>
      ))}
    </Fragment>
  ))
  return page(
    <ReportCard>
      <div className="report-scroll">
        <table className="report-table report-table--assets" aria-label={title}>
          <thead>
            <tr>
              <th scope="col">{t('reports.assets.description')}</th>
              <th scope="col">{t('reports.assets.purchaseDate')}</th>
              <th scope="col">{t('reports.assets.lifespan')}</th>
              {keys.map((k) => <th key={k} scope="col" className="report-table__amount">{t(`reports.assets.${k}`)}</th>)}
            </tr>
          </thead>
          {types.map((type) => (
            <Section key={type} title={t(`bookkeeping.accountTypes.${type}`)} span={span}>
              {rows(tree.filter((n) => accountType(n.id) === type), 0)}
            </Section>
          ))}
          <Section span={span}>
            <tr className="report-table__total is-grand">
              <th scope="row">{t('reports.total')}</th>
              <td /><td />
              {keys.map((_, i) => <td key={i} className="report-table__amount">{amount(sum(tree, keys.length)[i] ?? 0)}</td>)}
            </tr>
          </Section>
        </table>
      </div>
    </ReportCard>,
  )
}

/** Every category's balance at the start, its debits and credits, and its balance at the end of the period. */
export function TrialBalancePage() {
  const { t } = useTranslation()
  const f = useReportFilter('period')
  const ledger = useGeneralLedger(f.administrationId, { period: f.period })
  const accounts = useAccounts(f.administrationId)
  const title = t('journal.trialBalance')
  const values = useMemo(() => {
    const out = new Map<string, number[]>()
    const d = ledger.data
    if (!d) {
      return out
    }
    const lists = [d.start_balance, d.debit_sums, d.credit_sums, d.final_balance].map((v) => amounts(v))
    lists.forEach((m, i) => {
      for (const [id, v] of m) {
        const row = out.get(id) ?? [0, 0, 0, 0]
        row[i] = v
        out.set(id, row)
      }
    })
    return out
  }, [ledger.data])
  const tree = useMemo(() => {
    const type = (id: string) => accounts?.find((a) => a.id === id)?.account_type
    const all = categoryTree(values, accounts ?? [], 4, f.order, (name) => t('reports.general', { name }))
    return [...balanceSheetTypes, ...profitAndLossTypes].flatMap((ty) => all.filter((n) => type(n.id) === ty))
  }, [values, accounts, f.order, t])
  const page = (children: ReactNode) => <ReportPage filter={f} name={title} title={title} orderable>{children}</ReportPage>
  if (!ledger.data || !accounts) {
    return page(ledger.error ? <Failed /> : <Loading />)
  }
  if (tree.length === 0) {
    return page(<Empty f={f} />)
  }
  const columns: Column[] = [
    { key: 'start', label: <>{t('reports.trialBalance.opening')}<br />{lastDay(f.range.from - 1)}</> },
    { key: 'debit', label: t('journal.debit') },
    { key: 'credit', label: t('journal.credit') },
    { key: 'end', label: <>{t('reports.trialBalance.ending')}<br />{lastDay(f.range.to)}</> },
  ]
  const link = ledgerLink(f, [])
  const totals = sum(tree, 4)
  return page(
    <ReportCard>
      <p className="report-notice"><InfoIcon /><span>{t('reports.trialBalance.notice')}</span></p>
      <ReportTable label={title} columns={columns}>
        <Section span={5}>
          <CategoryRows nodes={tree} link={link} foldable={false} />
        </Section>
        <Section span={5}>
          <tr className="report-table__total is-grand">
            <th scope="row"><span className="sr-only">{t('journal.total')}</span></th>
            <td />
            <Amounts values={[totals[1] ?? 0, totals[2] ?? 0]} />
            <td />
          </tr>
        </Section>
      </ReportTable>
    </ReportCard>,
  )
}
