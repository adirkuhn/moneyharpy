import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { reportPeriods } from '../journal/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Pagination } from '../layout/Pagination'
import { filterTerms, invoiceTitle, type ListFilter, parseFilter, perPage, states, unpaidStates, useSalesInvoices } from './queries'
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
          <table className="data-table invoices" aria-label={t('invoices.title')}>
            <thead>
              <tr>
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
