import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors } from '../forms/Form'
import { Problems } from '../auth/AuthLayout'
import { reportPeriods } from '../journal/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Pagination } from '../layout/Pagination'
import {
  filterTerms, invoiceTitle, isPayable, type ListFilter, parseFilter, perPage, remindersFor, type SalesInvoice, states, unpaidStates,
  useDeleteDrafts, useRegisterFullPayments, useSalesInvoices, useSendReminders,
} from './queries'
import { StateBadge } from './StateBadge'

/** The quick filters at the top of Moneybird's invoice list. */
const quickFilters: Record<string, Pick<ListFilter, 'state' | 'period'>> = {
  this_year: { state: '', period: 'this_year' },
  draft: { state: 'draft', period: '' },
  unpaid: { state: unpaidStates, period: '' },
  paid: { state: 'paid', period: '' },
}

function quickOf(f: ListFilter) {
  const found = Object.entries(quickFilters).find(([, q]) => q.state === f.state && q.period === f.period && f.rest.length === 0)
  return found?.[0] ?? 'custom'
}

/** The sales invoices, filtered by state and period and paged (/sales_invoices). */
export function InvoicesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = parseFilter(params.get('filter'))
  const page = Math.max(1, Number(params.get('page')) || 1)
  const invoices = useSalesInvoices(administrationId, filterTerms(filter), page)
  const administration = useAdministration(administrationId)
  const quick = quickOf(filter)
  const base = `/${administrationId}/sales_invoices`
  const date = (d: string | null | undefined) => (d ? new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' }) : '')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const shown = invoices.data?.invoices ?? []
  const chosen = shown.filter((inv) => selected.has(inv.id ?? ''))
  const allChosen = shown.length > 0 && chosen.length === shown.length
  const toggle = (id: string) => {
    const next = new Set(selected)
    if (!next.delete(id)) {
      next.add(id)
    }
    setSelected(next)
  }

  const change = (changes: Partial<ListFilter>) => {
    const nextFilter = { ...filter, ...changes }
    const next = new URLSearchParams(params)
    if (quickOf(nextFilter) === 'this_year') {
      next.delete('filter')
    } else {
      next.set('filter', filterTerms(nextFilter))
    }
    next.delete('page')
    setParams(next, { replace: true })
    setSelected(new Set())
  }

  return (
    <Page title={t('invoices.title')} actions={<Link className="btn btn--accent" to={`${base}/new`}>{t('invoices.new')}</Link>}>
      <div className="filter-bar" role="search" aria-label={t('invoices.filters')}>
        <label className="filter-bar__field">
          <span>{t('invoices.show')}</span>
          <select value={quick} onChange={(e) => change(quickFilters[e.target.value] ?? {})}>
            {Object.keys(quickFilters).map((q) => <option key={q} value={q}>{t(`invoices.quick.${q}`)}</option>)}
            {quick === 'custom' && <option value="custom" disabled>{t('invoices.quick.custom')}</option>}
          </select>
        </label>
        <label className="filter-bar__field">
          <span>{t('invoices.period')}</span>
          <select value={filter.period} onChange={(e) => change({ period: e.target.value })}>
            <option value="">{t('invoices.allPeriods')}</option>
            {reportPeriods.map((p) => <option key={p} value={p}>{t(`journal.periods.${p}`)}</option>)}
          </select>
        </label>
        <label className="filter-bar__field">
          <span>{t('invoices.status')}</span>
          <select value={filter.state} onChange={(e) => change({ state: e.target.value })}>
            <option value="">{t('invoices.allStates')}</option>
            <option value={unpaidStates}>{t('invoices.quick.unpaid')}</option>
            {states.map((s) => <option key={s} value={s}>{t(`invoices.states.${s}`)}</option>)}
          </select>
        </label>
      </div>
      {!invoices.data ? (
        invoices.error ? <p className="empty">{t('invoices.loadError')}</p> : <Loading />
      ) : invoices.data.invoices.length === 0 ? (
        <p className="empty">{quick === 'this_year' && page === 1 ? t('invoices.empty') : t('invoices.noMatches')}</p>
      ) : (
        <>
          <BulkActions administrationId={administrationId} invoices={chosen} onDone={() => setSelected(new Set())} />
          <table className="data-table invoices" aria-label={t('invoices.title')}>
            <thead>
              <tr>
                <th scope="col" className="invoices__select">
                  <input type="checkbox" aria-label={t('invoices.bulk.selectAll')} checked={allChosen} onChange={() => setSelected(allChosen ? new Set() : new Set(shown.map((inv) => inv.id ?? '')))} />
                </th>
                <th scope="col" className="invoices__number">{t('invoices.number')}</th>
                <th scope="col" className="data-table__extra">{t('invoices.reference')}</th>
                <th scope="col" className="invoices__date data-table__extra">{t('invoices.invoiceDate')}</th>
                <th scope="col">{t('invoices.contact')}</th>
                <th scope="col" className="invoices__amount">{t('invoices.amount')}</th>
                <th scope="col" className="invoices__state">{t('invoices.status')}</th>
              </tr>
            </thead>
            <tbody>
              {invoices.data.invoices.map((inv) => (
                <tr key={inv.id}>
                  <td className="invoices__select">
                    <input type="checkbox" aria-label={t('invoices.bulk.select', { name: invoiceTitle(inv, t) })} checked={selected.has(inv.id ?? '')} onChange={() => toggle(inv.id ?? '')} />
                  </td>
                  <td className="invoices__number"><Link to={`${base}/${inv.id}`}>{invoiceTitle(inv, t)}</Link></td>
                  <td className="data-table__extra">{inv.reference}</td>
                  <td className="invoices__date data-table__extra">{date(inv.invoice_date)}</td>
                  <td>{inv.contact ? contactName(inv.contact) : ''}</td>
                  <td className="invoices__amount">{formatAmount(inv.total_price_incl_tax, inv.currency ?? administration.data?.currency ?? 'EUR', i18n.language)}</td>
                  <td className="invoices__state"><StateBadge state={inv.state} /></td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pagination total={invoices.data.total} perPage={perPage} />
        </>
      )}
    </Page>
  )
}

/** What can be done with the invoices ticked in the list at once. */
function BulkActions({ administrationId, invoices, onDone }: { administrationId: string; invoices: SalesInvoice[]; onDone: () => void }) {
  const { t } = useTranslation()
  const pay = useRegisterFullPayments(administrationId)
  const remind = useSendReminders(administrationId)
  const remove = useDeleteDrafts(administrationId)
  const [result, setResult] = useState('')
  const payable = invoices.filter((inv) => isPayable(inv.state))
  const drafts = invoices.filter((inv) => inv.state === 'draft')
  const busy = pay.isPending || remind.isPending || remove.isPending
  const report = (done: number, failed: SalesInvoice[], key: string) => {
    setResult(failed.length > 0 ? t('invoices.bulk.failed', { names: failed.map((inv) => invoiceTitle(inv, t)).join(', ') }) : t(key, { count: done }))
    onDone()
  }
  if (invoices.length === 0) {
    return result ? <p className="notice" role="status">{result}</p> : null
  }
  return (
    <div className="filter-bar invoices__bulk" role="group" aria-label={t('invoices.bulk.label')}>
      <span role="status">{t('invoices.bulk.selected', { count: invoices.length })}</span>
      <Problems messages={formErrors(remind.error, {}).general} />
      <button type="button" className="btn btn--secondary btn--small" disabled={busy || payable.length === 0}
        onClick={() => pay.mutate(payable, { onSuccess: (failed) => report(payable.length - failed.length, failed, 'invoices.bulk.paid') })}>
        {t('invoices.bulk.pay', { count: payable.length })}
      </button>
      <button type="button" className="btn btn--secondary btn--small" disabled={busy || payable.length === 0}
        onClick={() => remind.mutate(remindersFor(payable), { onSuccess: (list) => report(list.length, [], 'invoices.bulk.reminded') })}>
        {t('invoices.bulk.remind', { count: payable.length })}
      </button>
      {drafts.length > 0 && (
        <ConfirmDelete
          label={t('invoices.bulk.delete', { count: drafts.length })}
          confirmLabel={t('invoices.bulk.confirmDelete', { count: drafts.length })}
          disabled={busy}
          onConfirm={() => remove.mutate(drafts, { onSuccess: (failed) => report(drafts.length - failed.length, failed, 'invoices.bulk.deleted') })}
        />
      )}
    </div>
  )
}
