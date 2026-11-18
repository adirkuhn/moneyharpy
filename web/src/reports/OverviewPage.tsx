import { type ReactNode, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { contactName, useContact } from '../contacts/queries'
import { ChevronIcon, InfoIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { BarsChart, Legend, LinesChart, RingChart } from './charts'
import { partName, rangeName } from './period'
import {
  type Subject, useBankBalance, useCashFlow, useContactReport, useProfitLoss, useTaxReport, useVatOverview, useWorkingCapital,
} from './queries'
import { apiGranularity, type ReportFilter, ReportPage, useReportFilter } from './ReportFrame'

/** The first and the last day of the period, as the VAT overview takes them. */
function vatDays(f: ReportFilter): string {
  const first = new Date(Date.UTC(Math.floor(f.range.from / 12), f.range.from % 12, 1))
  const last = new Date(Date.UTC(Math.floor(f.range.to / 12), (f.range.to % 12) + 1, 0))
  const digits = (d: Date) => d.toISOString().slice(0, 10).replaceAll('-', '')
  return `${digits(first)}..${digits(last)}`
}

/**
 * Moneybird's reports overview: for the period, a card each for the profit,
 * the working capital, the VAT, the bank balance, what customers owe and
 * what is owed to suppliers, each with its amount, a chart, what makes it up
 * and a link to the report behind it.
 */
export function ReportsOverviewPage() {
  const { t } = useTranslation()
  const f = useReportFilter('granular')
  const title = t('sidenav.items.reportsOverview')
  return (
    <ReportPage filter={f} name={t('reports.overview.name')} title={title} exportable={false} info={{
      title: t('reports.overview.info.title'),
      text: t('reports.overview.info.text'),
      steps: [0, 1, 2].map((i) => ({ icon: (['chart', 'check', 'calendar'] as const)[i] ?? 'chart', title: t(`reports.overview.info.steps.${i}.title`), text: t(`reports.overview.info.steps.${i}.text`) })),
    }}>
      <div className="report-overview">
        <ProfitCard f={f} />
        <WorkingCapitalCard f={f} />
        <VatCard f={f} />
        <BankCard f={f} />
        <SubjectsCard f={f} kind="debtors" />
        <SubjectsCard f={f} kind="creditors" />
      </div>
    </ReportPage>
  )
}

function useMoney(administrationId: string) {
  const { i18n } = useTranslation()
  const currency = useAdministration(administrationId).data?.currency ?? 'EUR'
  return (n: number | string) => formatAmount(n, currency, i18n.language)
}

function useParts(parts: { period?: string }[] | undefined) {
  const { t, i18n } = useTranslation()
  return (parts ?? []).map((p) => partName(p.period ?? '', t, i18n.language))
}

type CardProps = { title: ReactNode; link: { to: string; label: string }; amount?: string; children?: ReactNode }

function Card({ title, link, amount, children }: CardProps) {
  return (
    <section className="overview-card">
      <div className="overview-card__head">
        <h2>{title}</h2>
        <Link to={link.to}>{link.label}<ChevronIcon /></Link>
      </div>
      {amount === undefined ? <Loading /> : (
        <>
          <p className="overview-card__amount">{amount}</p>
          {children}
        </>
      )}
    </section>
  )
}

const report = (f: ReportFilter, slug: string) => `/${f.administrationId}/report/${slug}?period=${f.period}${f.granularity && f.granularity !== 'none' ? `&granularity=${f.granularity}` : ''}`

function ProfitCard({ f }: { f: ReportFilter }) {
  const { t } = useTranslation()
  const money = useMoney(f.administrationId)
  const r = useProfitLoss(f.administrationId, { period: f.period, granularity: apiGranularity(f) }).data
  const parts = r?.periods ?? []
  const categories = useParts(parts)
  const n = (v?: string) => Number(v ?? 0)
  const direct = (p: { total_revenue?: string; gross_profit?: string }) => n(p.total_revenue) - n(p.gross_profit)
  return (
    <Card title={t('reports.overview.profit')} link={{ to: report(f, 'profit_loss'), label: t('sidenav.items.profitLoss') }} amount={r && money(r.net_profit ?? 0)}>
      {r && (
        <>
          <BarsChart
            label={t('reports.overview.profit')}
            categories={categories}
            series={[
              { key: 'revenue', label: t('reports.overview.revenue'), values: parts.map((p) => n(p.total_revenue)), color: 1 },
              { key: 'direct', label: t('reports.overview.directCosts'), values: parts.map(direct), color: 2 },
              { key: 'expenses', label: t('reports.overview.expenses'), values: parts.map((p) => n(p.total_expenses)), color: 3 },
            ]}
          />
          <Legend items={[
            { key: 'revenue', label: t('reports.overview.revenue'), value: money(r.total_revenue ?? 0), color: 1 },
            { key: 'direct', label: t('reports.overview.directCosts'), value: money(direct(r)), color: 2 },
            { key: 'expenses', label: t('reports.overview.expenses'), value: money(r.total_expenses ?? 0), color: 3 },
          ]} />
        </>
      )}
    </Card>
  )
}

/** A question mark that explains the card's amount on hover or focus. */
function Explained({ title, text }: { title: string; text: string }) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <>
      {title}
      <span className="overview-card__tip">
        <button type="button" aria-label={t('reports.overview.explain', { title })} aria-describedby={id}><InfoIcon /></button>
        <span id={id} role="tooltip">{text}</span>
      </span>
    </>
  )
}

function WorkingCapitalCard({ f }: { f: ReportFilter }) {
  const { t } = useTranslation()
  const money = useMoney(f.administrationId)
  const r = useWorkingCapital(f.administrationId, { period: f.period, granularity: apiGranularity(f) }).data
  const parts = r?.periods ?? []
  const categories = useParts(parts)
  return (
    <Card
      title={<Explained title={t('reports.overview.workingCapital')} text={t('reports.overview.workingCapitalHelp')} />}
      link={{ to: report(f, 'balance_sheet'), label: t('sidenav.items.balanceSheet') }}
      amount={r && money(r.working_capital ?? 0)}
    >
      {r && (
        <>
          <LinesChart
            label={t('reports.overview.workingCapital')}
            categories={categories}
            series={[{ key: 'capital', label: t('reports.overview.workingCapital'), values: parts.map((p) => Number(p.working_capital ?? 0)), color: 1 }]}
          />
          <Legend items={[
            { key: 'bank', label: t('reports.overview.bankBalance'), value: money(r.bank ?? 0), color: 1 },
            { key: 'receive', label: t('reports.overview.receivables'), value: money(r.to_receive ?? 0), color: 2 },
            { key: 'pay', label: t('reports.overview.liabilities'), value: money(r.to_pay ?? 0), color: 3 },
          ]} />
        </>
      )}
    </Card>
  )
}

function VatCard({ f }: { f: ReportFilter }) {
  const { t } = useTranslation()
  const money = useMoney(f.administrationId)
  const overview = useVatOverview(f.administrationId, 'vat_return', vatDays(f), true).data
  const rates = useTaxReport(f.administrationId, { period: f.period, granularity: apiGranularity(f) }).data
  const parts = rates?.periods ?? []
  const categories = useParts(parts)
  const sum = (p: (typeof parts)[number], type: string) => (p.tax_rates ?? []).filter((r) => r.type === type).reduce((s, r) => s + Math.abs(Number(r.tax ?? 0)), 0)
  return (
    <Card title={t('reports.overview.vat')} link={{ to: `/${f.administrationId}/report/tax?period=${f.period}`, label: t('reports.overview.vatLink') }} amount={overview && money(overview.total)}>
      {overview && (
        <>
          <BarsChart
            label={t('reports.overview.vat')}
            categories={categories}
            series={[
              { key: 'payable', label: t('reports.overview.payableVat'), values: parts.map((p) => sum(p, 'sales_invoice')), color: 1 },
              { key: 'receivable', label: t('reports.overview.receivableVat'), values: parts.map((p) => sum(p, 'purchase_invoice')), color: 2 },
            ]}
          />
          <Legend items={[
            { key: 'payable', label: t('reports.overview.payableVat'), value: money(overview.owed), color: 1 },
            { key: 'receivable', label: t('reports.overview.receivableVat'), value: money(overview.input_tax), color: 2 },
          ]} />
        </>
      )}
    </Card>
  )
}

function BankCard({ f }: { f: ReportFilter }) {
  const { t, i18n } = useTranslation()
  const money = useMoney(f.administrationId)
  const balance = useBankBalance(f.administrationId, { period: f.period, granularity: apiGranularity(f) }).data
  const flow = useCashFlow(f.administrationId, { period: f.period }).data
  const parts = balance?.periods ?? []
  const categories = useParts(parts)
  const total = (values?: { ledger_accounts?: { value?: string }[] }) => (values?.ledger_accounts ?? []).reduce((s, v) => s + Number(v.value ?? 0), 0)
  const start = new Date(Math.floor(f.range.from / 12), f.range.from % 12, 1).toLocaleDateString(i18n.language, { day: '2-digit', month: 'long', year: 'numeric' })
  return (
    <Card title={t('reports.overview.bankBalance')} link={{ to: report(f, 'cash_flow'), label: t('sidenav.items.cashFlow') }} amount={balance && flow && money(balance.total ?? 0)}>
      {balance && flow && (
        <>
          <LinesChart
            label={t('reports.overview.bankBalance')}
            categories={categories}
            series={[{ key: 'balance', label: t('reports.overview.bankBalance'), values: parts.map((p) => Number(p.total ?? 0)), color: 1 }]}
          />
          <Legend items={[
            { key: 'opening', label: t('reports.overview.openingOn', { date: start }), value: money(flow.opening_balance ?? 0), color: 1 },
            { key: 'credited', label: t('reports.overview.credited'), value: money(total(flow.cash_received_by_ledger_account)), color: 2 },
            { key: 'debited', label: t('reports.overview.debited'), value: money(Math.abs(total(flow.cash_paid_by_ledger_account))), color: 3 },
          ]} />
        </>
      )}
    </Card>
  )
}

/** The names of up to five contacts. */
function useNames(administrationId: string, ids: (string | null)[]) {
  const { t } = useTranslation()
  const contacts = [
    useContact(administrationId, ids[0] ?? undefined), useContact(administrationId, ids[1] ?? undefined), useContact(administrationId, ids[2] ?? undefined),
    useContact(administrationId, ids[3] ?? undefined), useContact(administrationId, ids[4] ?? undefined),
  ]
  return ids.map((id, i) => {
    const c = contacts[i]?.data
    return id == null ? t('reports.noContact') : c ? contactName(c) : ''
  })
}

/** What customers owe or what is owed to suppliers at the end of the period: the five largest and the rest. */
function SubjectsCard({ f, kind }: { f: ReportFilter; kind: 'debtors' | 'creditors' }) {
  const { t, i18n } = useTranslation()
  const money = useMoney(f.administrationId)
  const r = useContactReport(f.administrationId, kind, { period: f.period }, 1).data
  const rows: Subject[] = (r?.rows ?? []).map((x) => ({ ...x, amount: String(Math.abs(Number(x.amount))) }))
  const top = rows.slice(0, 5)
  const other = rows.slice(5).reduce((s, x) => s + Number(x.amount), 0)
  const all = rows.reduce((s, x) => s + Number(x.amount), 0)
  const name = t(kind === 'debtors' ? 'reports.overview.outstanding' : 'reports.overview.owed')
  const names = useNames(f.administrationId, top.map((x) => x.id))
  const parts = [
    ...top.map((x, i) => ({ key: String(x.id), label: names[i] ?? '', value: Number(x.amount), color: i + 1 })),
    ...(other > 0 ? [{ key: 'other', label: t('reports.overview.other'), value: other, color: 0 }] : []),
  ]
  return (
    <Card
      title={name}
      link={{ to: report(f, kind), label: t(kind === 'debtors' ? 'reports.overview.agedReceivables' : 'reports.overview.agedPayables') }}
      amount={r && money(all)}
    >
      {r && (
        <>
          <RingChart label={`${name}, ${rangeName(f.range, t, i18n.language)}`} parts={parts} />
          {parts.length > 0
            ? <Legend items={parts.map((p) => ({ key: p.key, label: p.label, value: money(p.value), color: p.color }))} />
            : <p className="muted">{t('reports.overview.nobody')}</p>}
        </>
      )}
    </Card>
  )
}
