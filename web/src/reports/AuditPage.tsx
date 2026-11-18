import { Fragment, type ReactNode, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { balanceSheetTypes, profitAndLossTypes, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { useUsers } from '../management/queries'
import { lastDay, rangeName } from './period'
import { amounts, type AuditLine, useAuditReport, useGeneralLedger, useToggleAudit } from './queries'
import { type ReportFilter, ReportPage, useReportFilter } from './ReportFrame'
import { categoryTree, type TreeNode, useAmount } from './ReportTable'

/** Whether the audit report counts every audit or only those by bookkeepers, after Moneybird's "Display". */
type Display = 'all' | 'accountants'

/**
 * Moneybird's audit report: the trial balance of a period with, per
 * category, the VAT rates booked on it and a button to mark it as audited.
 * The card says how many categories are audited; an audit made before the
 * bookings changed is flagged.
 */
export function AuditReportPage() {
  const { t, i18n } = useTranslation()
  const f = useReportFilter('period')
  const display: Display = f.format === 'accountants' ? 'accountants' : 'all'
  const ledger = useGeneralLedger(f.administrationId, { period: f.period })
  const audit = useAuditReport(f.administrationId, f.period)
  const accounts = useLedgerAccounts(f.administrationId).data
  const rates = useTaxRates(f.administrationId).data
  const users = useUsers(f.administrationId).data
  const title = t('sidenav.items.auditReport')
  const what = rangeName(f.range, t, i18n.language)

  const values = useMemo(() => {
    const out = new Map<string, number[]>()
    const d = ledger.data
    if (!d) {
      return out
    }
    ;[d.start_balance, d.debit_sums, d.credit_sums, d.final_balance].map((v) => amounts(v)).forEach((m, i) => {
      for (const [id, v] of m) {
        const row = out.get(id) ?? [0, 0, 0, 0]
        row[i] = v
        out.set(id, row)
      }
    })
    for (const l of audit.data?.ledger_accounts ?? []) {
      if (!out.has(l.ledger_account_id)) {
        out.set(l.ledger_account_id, [0, 0, 0, 0])
      }
    }
    return out
  }, [ledger.data, audit.data])
  const sections = useMemo(() => {
    const all = categoryTree(values, accounts ?? [], 4, f.order, (name) => t('reports.general', { name }))
    const type = (id: string) => accounts?.find((a) => a.id === id)?.account_type
    const of = (types: string[]) => all.filter((n) => types.includes(type(n.id) ?? ''))
    return [
      { key: 'balance', title: t('reports.audit.balanceSheet'), nodes: of(balanceSheetTypes) },
      { key: 'profit', title: t('reports.audit.profitLoss'), nodes: of(profitAndLossTypes) },
    ].filter((s) => s.nodes.length > 0)
  }, [values, accounts, f.order, t])

  const accountants = useMemo(() => new Set((users ?? []).filter((u) => u.user_type === 'accountant').map((u) => String(u.id))), [users])
  const lines = useMemo(() => {
    const out = new Map<string, AuditLine>()
    for (const l of audit.data?.ledger_accounts ?? []) {
      const counts = l.audit && (display === 'all' || accountants.has(l.audit.user_id))
      out.set(l.ledger_account_id, counts ? l : { ...l, audit: null, changed: false })
    }
    return out
  }, [audit.data, display, accountants])

  const page = (children: ReactNode) => (
    <ReportPage
      filter={f}
      name={title}
      title={title}
      orderable
      exportable={false}
      formats={{ legend: t('reports.audit.display'), options: [{ value: 'all', label: t('reports.audit.all') }, { value: 'accountants', label: t('reports.audit.accountants') }] }}
      info={{
        title: t('reports.audit.info.title'),
        text: t('reports.audit.info.text'),
        steps: [0, 1, 2].map((i) => ({ icon: (['check', 'clock', 'edit'] as const)[i] ?? 'check', title: t(`reports.audit.info.steps.${i}.title`), text: t(`reports.audit.info.steps.${i}.text`) })),
      }}
    >
      {children}
    </ReportPage>
  )
  if (!ledger.data || !audit.data || !accounts) {
    return page(ledger.error || audit.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />)
  }
  // What can be audited: the categories without sub-categories, and the own bookings of those with.
  const leaves = sections.flatMap((s) => leavesOf(s.nodes))
  const done = leaves.filter((n) => lines.get(n.id)?.audit).length
  const rateName = (id: string) => rates?.find((r) => r.id === id)?.name ?? ''
  return page(
    <section className="report-card audit-card">
      <div className="audit-progress">
        <ListIcon kind="check" />
        <strong>{t('reports.audit.label.period', { name: title, period: what })}</strong>
        <span>{t('reports.audit.progress', { done, total: leaves.length })}</span>
        <progress max={Math.max(leaves.length, 1)} value={done} aria-label={t('reports.audit.progressLabel')} />
      </div>
      <div className="report-scroll">
        <table className="report-table audit-table" aria-label={title}>
          <thead>
            <tr>
              <th scope="col"><span className="sr-only">{t('journal.category')}</span></th>
              <th scope="col" className="report-table__amount">{t('reports.trialBalance.opening')}<br /><span className="audit-date">{lastDay(f.range.from - 1)}</span></th>
              <th scope="col" className="report-table__amount">{t('journal.debit')}</th>
              <th scope="col" className="report-table__amount">{t('journal.credit')}</th>
              <th scope="col" className="report-table__amount">{t('reports.trialBalance.ending')}<br /><span className="audit-date">{lastDay(f.range.to)}</span></th>
              <th scope="col">{t('reports.audit.vat')}</th>
              <th scope="col"><span className="sr-only">{t('reports.audit.audited')}</span></th>
            </tr>
          </thead>
          {sections.map((s) => (
            <tbody key={s.key}>
              <tr className="audit-table__section"><th scope="rowgroup" colSpan={7}>{s.title}</th></tr>
              <Rows f={f} nodes={s.nodes} lines={lines} rateName={rateName} users={users ?? []} />
            </tbody>
          ))}
          <tbody>
            <tr className="report-table__total is-grand">
              <th scope="row"><span className="sr-only">{t('journal.total')}</span></th>
              <td />
              <td className="report-table__amount"><Amount v={sum(sections, 1)} /></td>
              <td className="report-table__amount"><Amount v={sum(sections, 2)} /></td>
              <td /><td /><td />
            </tr>
          </tbody>
        </table>
      </div>
    </section>,
  )
}

function leavesOf(nodes: TreeNode[]): TreeNode[] {
  return nodes.flatMap((n) => (n.children.length > 0 ? leavesOf(n.children) : [n]))
}

const sum = (sections: { nodes: TreeNode[] }[], column: number) =>
  sections.reduce((s, x) => s + x.nodes.reduce((a, n) => a + (n.values[column] ?? 0), 0), 0)

function Amount({ v }: { v: number }) {
  const amount = useAmount()
  return <>{amount(v)}</>
}

type RowsProps = {
  f: ReportFilter
  nodes: TreeNode[]
  lines: Map<string, AuditLine>
  rateName: (id: string) => string
  users: { id?: string; name?: string }[]
  depth?: number
}

function Rows({ f, nodes, lines, rateName, users, depth = 0 }: RowsProps) {
  const amount = useAmount()
  return (
    <>
      {nodes.map((n) => {
        const leaf = n.children.length === 0
        const line = lines.get(n.id)
        const rates = [...new Set((line?.tax_rate_ids ?? []).map(rateName).filter(Boolean))]
        const link = `/${f.administrationId}/report/journal_entries?ledger_account_id=${n.id}&period=${f.period}`
        return (
          <Fragment key={n.key}>
            <tr className={`audit-table__row is-depth-${Math.min(depth, 3)}${leaf ? ' is-leaf' : ''}`}>
              <th scope="row">
                <span className="report-table__name">
                  {leaf ? <Link to={link}>{n.name}</Link> : <span>{n.name}</span>}
                  {n.code && <>{' '}<span className="report-table__code">{n.code}</span></>}
                </span>
              </th>
              {n.values.map((v, i) => (
                <td key={i} className="report-table__amount">{leaf && v !== 0 ? <Link to={link}>{amount(v)}</Link> : amount(v)}</td>
              ))}
              <td className="audit-table__rates">{rates.map((r) => <span key={r} className="audit-rate">{r}</span>)}</td>
              <td className="audit-table__check">{leaf && <AuditButton f={f} id={n.id} name={n.name} line={line} users={users} />}</td>
            </tr>
            {!leaf && <Rows f={f} nodes={n.children} lines={lines} rateName={rateName} users={users} depth={depth + 1} />}
          </Fragment>
        )
      })}
    </>
  )
}

/** The round button that marks a category as audited, says by whom and when, and flags changes since. */
function AuditButton({ f, id, name, line, users }: { f: ReportFilter; id: string; name: string; line?: AuditLine; users: { id?: string; name?: string }[] }) {
  const { t, i18n } = useTranslation()
  const toggle = useToggleAudit(f.administrationId, f.period)
  const a = line?.audit
  const who = users.find((u) => String(u.id) === a?.user_id)?.name ?? ''
  const when = a ? new Date(a.audited_at).toLocaleDateString(i18n.language) : ''
  const label = a
    ? line?.changed ? t('reports.audit.changed', { name, user: who, date: when }) : t('reports.audit.auditedBy', { name, user: who, date: when })
    : t('reports.audit.mark', { name })
  return (
    <button
      type="button"
      className={`audit-check${a ? ' is-done' : ''}${line?.changed ? ' is-changed' : ''}`}
      aria-pressed={!!a}
      aria-label={label}
      title={label}
      disabled={toggle.isPending}
      onClick={() => toggle.mutate({ ledgerAccountId: id, audited: !!a })}
    >
      {line?.changed ? <span aria-hidden="true">!</span> : a ? <ListIcon kind="check" /> : null}
    </button>
  )
}
