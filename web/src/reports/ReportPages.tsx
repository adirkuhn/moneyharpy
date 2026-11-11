import { type ReactNode, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { type LedgerAccount, useLedgerAccounts } from '../bookkeeping/queries'
import { contactName, useContact } from '../contacts/queries'
import { reportPeriods } from '../journal/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { PeriodFilter } from '../list/PeriodFilter'
import { isPeriod } from '../list/period'
import {
  type AgingKind, amounts, type ContactReportKind, useAgingReport, useBalanceSheet, useCashFlow, useContactReport, useProfitLoss, useTaxReport,
} from './queries'

type Frame = {
  administrationId: string
  period: string
  money: (n: number | string) => string
  /** The page around the report, with the period filter unless the report takes no period. */
  page: (children: ReactNode) => ReactNode
}

/** The frame every report shares: its title, the period filter of the address and the administration's currency. */
function useFrame(slug: string, title: string, withPeriod = true): Frame {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const asked = search.get('period') ?? ''
  const period = isPeriod(asked, today) ? asked : 'this_month'
  const administration = useAdministration(administrationId)
  const currency = administration.data?.currency ?? 'EUR'
  const base = `/${administrationId}/report/${slug}`
  const page = (children: ReactNode) => (
    <Page
      wide
      title={title}
      lead={withPeriod ? (
        <PeriodFilter
          period={period}
          today={today}
          title={t('journal.filters')}
          quick={reportPeriods.map((p) => ({ value: p, label: t(`journal.periods.${p}`) }))}
          linkTo={(p) => `${base}?period=${p}`}
          onApply={(p) => setSearch({ period: p }, { replace: true })}
        />
      ) : undefined}
    >
      {children}
    </Page>
  )
  return { administrationId, period, money: (n) => formatAmount(n, currency, i18n.language), page }
}

function AccountName({ id, accounts }: { id: string; accounts: Map<string, LedgerAccount> }) {
  const a = accounts.get(id)
  return (
    <>
      {a?.account_id && <span className="code-chip">{a.account_id}</span>}
      {a?.name ?? id}
    </>
  )
}

function useAccounts(administrationId: string) {
  const accounts = useLedgerAccounts(administrationId)
  return useMemo(() => new Map((accounts.data ?? []).map((a) => [a.id ?? '', a])), [accounts.data])
}

/** A titled group of category rows with an amount each. */
function Section({ title, rows, accounts, money }: { title: string; rows: [string, number][]; accounts: Map<string, LedgerAccount>; money: Frame['money'] }) {
  return (
    <tbody>
      <tr className="category-table__group">
        <th scope="rowgroup" colSpan={2}>{title}</th>
      </tr>
      {rows.map(([id, value]) => (
        <tr key={id}>
          <td><AccountName id={id} accounts={accounts} /></td>
          <td className="ledger-report__amount">{money(value)}</td>
        </tr>
      ))}
    </tbody>
  )
}

function TotalRow({ label, value, money }: { label: string; value?: string; money: Frame['money'] }) {
  return (
    <tr className="ledger-report__total">
      <th scope="row">{label}</th>
      <td className="ledger-report__amount">{money(value ?? 0)}</td>
    </tr>
  )
}

/** The profit and loss categories with bookings in the period, and the profits. */
export function ProfitLossPage() {
  const { t } = useTranslation()
  const f = useFrame('profit_loss', t('sidenav.items.profitLoss'))
  const report = useProfitLoss(f.administrationId, f.period)
  const accounts = useAccounts(f.administrationId)
  if (!report.data) {
    return f.page(report.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />)
  }
  const r = report.data
  return f.page(
    <table className="data-table ledger-report" aria-label={t('sidenav.items.profitLoss')}>
      <Section title={t('bookkeeping.accountTypes.revenue')} rows={amounts(r.revenue_by_ledger_account)} accounts={accounts} money={f.money} />
      <tbody><TotalRow label={t('reports.totalRevenue')} value={r.total_revenue} money={f.money} /></tbody>
      <Section title={t('bookkeeping.accountTypes.direct_costs')} rows={amounts(r.direct_costs_by_ledger_account)} accounts={accounts} money={f.money} />
      <tbody><TotalRow label={t('reports.grossProfit')} value={r.gross_profit} money={f.money} /></tbody>
      <Section title={t('bookkeeping.accountTypes.expenses')} rows={amounts(r.expenses_by_ledger_account)} accounts={accounts} money={f.money} />
      <tbody><TotalRow label={t('reports.operatingProfit')} value={r.operating_profit} money={f.money} /></tbody>
      <Section title={t('bookkeeping.accountTypes.other_income_expenses')} rows={amounts(r.other_income_expenses_by_ledger_account)} accounts={accounts} money={f.money} />
      <tfoot><TotalRow label={t('reports.netProfit')} value={r.net_profit} money={f.money} /></tfoot>
    </table>,
  )
}

type BalanceLines = { ledger_account_id?: string; value?: string; children?: { ledger_account_id?: string; value?: string }[] }[] | undefined

function flat(lines: BalanceLines): [string, number][] {
  return (lines ?? []).flatMap((l) => [
    [l.ledger_account_id ?? '', Number(l.value ?? 0)] as [string, number],
    ...(l.children ?? []).map((c) => [c.ledger_account_id ?? '', Number(c.value ?? 0)] as [string, number]),
  ])
}

/** The balance sheet at the end of the period: assets against equity and liabilities. */
export function BalanceSheetPage() {
  const { t } = useTranslation()
  const f = useFrame('balance_sheet', t('sidenav.items.balanceSheet'))
  const report = useBalanceSheet(f.administrationId, f.period)
  const accounts = useAccounts(f.administrationId)
  if (!report.data) {
    return f.page(report.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />)
  }
  const { debit, credit } = report.data
  const section = (type: string, lines: BalanceLines) => (
    flat(lines).length > 0 && <Section key={type} title={t(`bookkeeping.accountTypes.${type}`)} rows={flat(lines)} accounts={accounts} money={f.money} />
  )
  return f.page(
    <>
      <table className="data-table ledger-report" aria-label={t('reports.debit')}>
        {section('non_current_assets', debit?.non_current_assets)}
        {section('current_assets', debit?.current_assets)}
        <tfoot><TotalRow label={t('reports.totalDebit')} value={debit?.total} money={f.money} /></tfoot>
      </table>
      <table className="data-table ledger-report" aria-label={t('reports.credit')}>
        {section('equity', credit?.equity)}
        {section('provisions', credit?.provisions)}
        {section('non_current_liabilities', credit?.non_current_liabilities)}
        {section('current_liabilities', credit?.current_liabilities)}
        {(credit?.open_years ?? []).length > 0 && (
          <tbody>
            <tr className="category-table__group"><th scope="rowgroup" colSpan={2}>{t('reports.openYears')}</th></tr>
            {(credit?.open_years ?? []).map((y) => (
              <tr key={y.year}><td>{y.year}</td><td className="ledger-report__amount">{f.money(y.value ?? 0)}</td></tr>
            ))}
          </tbody>
        )}
        <tfoot><TotalRow label={t('reports.totalCredit')} value={credit?.total} money={f.money} /></tfoot>
      </table>
    </>,
  )
}

/** Per VAT rate what it was charged on and the VAT, in the period. */
export function TaxReportPage() {
  const { t } = useTranslation()
  const f = useFrame('tax', t('sidenav.items.vatSummary'))
  const report = useTaxReport(f.administrationId, f.period)
  if (!report.data) {
    return f.page(report.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />)
  }
  const rates = report.data.tax_rates ?? []
  if (rates.length === 0) {
    return f.page(<p className="empty">{t('journal.nothingBooked')}</p>)
  }
  return f.page(
    <table className="data-table ledger-report" aria-label={t('sidenav.items.vatSummary')}>
      <thead>
        <tr>
          <th scope="col">{t('reports.taxRate')}</th>
          <th scope="col" className="data-table__extra">{t('reports.reference')}</th>
          <th scope="col" className="ledger-report__amount">{t('reports.turnover')}</th>
          <th scope="col" className="ledger-report__amount">{t('reports.vat')}</th>
        </tr>
      </thead>
      <tbody>
        {rates.map((r) => (
          <tr key={`${r.tax_rate_id}`}>
            <td>{r.name} <span className="muted">({t(`reports.rateTypes.${r.type === 'sales_invoice' ? 'sales' : 'purchases'}`)})</span></td>
            <td className="data-table__extra">{r.report_reference}</td>
            <td className="ledger-report__amount">{f.money(r.amount ?? 0)}</td>
            <td className="ledger-report__amount">{f.money(r.tax ?? 0)}</td>
          </tr>
        ))}
      </tbody>
    </table>,
  )
}

/** The cash received and paid in the period, between the opening and the closing balance. */
export function CashFlowPage() {
  const { t } = useTranslation()
  const f = useFrame('cash_flow', t('sidenav.items.cashFlow'))
  const report = useCashFlow(f.administrationId, f.period)
  const accounts = useAccounts(f.administrationId)
  if (!report.data) {
    return f.page(report.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />)
  }
  const r = report.data
  return f.page(
    <table className="data-table ledger-report" aria-label={t('sidenav.items.cashFlow')}>
      <tbody><TotalRow label={t('reports.openingBalance')} value={r.opening_balance} money={f.money} /></tbody>
      <Section title={t('reports.received')} rows={amounts(r.cash_received_by_ledger_account)} accounts={accounts} money={f.money} />
      <Section title={t('reports.paid')} rows={amounts(r.cash_paid_by_ledger_account)} accounts={accounts} money={f.money} />
      <tfoot><TotalRow label={t('reports.closingBalance')} value={r.closing_balance} money={f.money} /></tfoot>
    </table>,
  )
}

function ContactCell({ administrationId, id }: { administrationId: string; id?: string | number | null }) {
  const { t } = useTranslation()
  const contact = useContact(administrationId, id == null ? undefined : String(id))
  if (id == null) {
    return <>{t('reports.noContact')}</>
  }
  return <Link to={`/${administrationId}/contacts/${id}`}>{contact.data ? contactName(contact.data) : String(id)}</Link>
}

const contactTitles: Record<ContactReportKind, 'debtors' | 'creditors' | 'revenueByContact' | 'expensesByContact'> = {
  debtors: 'debtors', creditors: 'creditors', revenue_by_contact: 'revenueByContact', expenses_by_contact: 'expensesByContact',
}

/** An amount per contact: the balances of debtors or creditors, or the revenue or costs per contact. */
export function ContactReportPage({ kind }: { kind: ContactReportKind }) {
  const { t } = useTranslation()
  const title = t(`sidenav.items.${contactTitles[kind]}`)
  const f = useFrame(kind, title)
  const report = useContactReport(f.administrationId, kind, f.period)
  if (!report.data) {
    return f.page(report.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />)
  }
  if (report.data.length === 0) {
    return f.page(<p className="empty">{t('reports.nothing')}</p>)
  }
  const total = report.data.reduce((sum, x) => sum + Number(x.amount), 0)
  return f.page(
    <table className="data-table ledger-report" aria-label={title}>
      <thead>
        <tr>
          <th scope="col">{t('reports.contact')}</th>
          <th scope="col" className="ledger-report__amount">{t('journal.amount')}</th>
        </tr>
      </thead>
      <tbody>
        {report.data.map((x) => (
          <tr key={String(x.contact_id)}>
            <td><ContactCell administrationId={f.administrationId} id={x.contact_id} /></td>
            <td className="ledger-report__amount">{f.money(x.amount)}</td>
          </tr>
        ))}
      </tbody>
      <tfoot><TotalRow label={t('journal.total')} value={String(total)} money={f.money} /></tfoot>
    </table>,
  )
}

/** What each contact has open today, by age. */
export function AgingReportPage({ kind }: { kind: AgingKind }) {
  const { t } = useTranslation()
  const title = t(kind === 'debtors_aging' ? 'sidenav.items.debtorsAging' : 'sidenav.items.creditorsAging')
  const f = useFrame(kind, title, false)
  const report = useAgingReport(f.administrationId, kind, 'this_month')
  if (!report.data) {
    return f.page(report.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />)
  }
  if (report.data.length === 0) {
    return f.page(<p className="empty">{t('reports.nothingOpen')}</p>)
  }
  const names = report.data[0]?.aging_buckets?.map((b) => b.name ?? '') ?? []
  return f.page(
    <table className="data-table ledger-report" aria-label={title}>
      <thead>
        <tr>
          <th scope="col">{t('reports.contact')}</th>
          {names.map((n, i) => <th key={i} scope="col" className="ledger-report__amount">{n}</th>)}
        </tr>
      </thead>
      <tbody>
        {report.data.map((x) => (
          <tr key={String(x.contact_id)}>
            <td><ContactCell administrationId={f.administrationId} id={x.contact_id} /></td>
            {(x.aging_buckets ?? []).map((b, i) => <td key={i} className="ledger-report__amount">{f.money(b.amount ?? 0)}</td>)}
          </tr>
        ))}
      </tbody>
    </table>,
  )
}
