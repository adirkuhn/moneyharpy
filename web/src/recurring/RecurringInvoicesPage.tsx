import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { perPage } from '../invoices/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Pagination } from '../layout/Pagination'
import { frequencyText, frequencyTypes, type RecurringFilter, recurringFilterTerms, useRecurringInvoices } from './queries'

/** Recurring invoices, active ones unless filtered otherwise (/recurring_sales_invoices). */
export function RecurringInvoicesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter: RecurringFilter = { state: params.get('state') ?? 'active', frequency: params.get('frequency') ?? '', autoSend: params.get('auto_send') ?? '' }
  const page = Math.max(1, Number(params.get('page')) || 1)
  const invoices = useRecurringInvoices(administrationId, recurringFilterTerms(filter), page)
  const base = `/${administrationId}/recurring_sales_invoices`
  const date = (d: string | null | undefined) => (d ? new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' }) : '')
  const isDefault = filter.state === 'active' && !filter.frequency && !filter.autoSend

  const change = (key: 'state' | 'frequency' | 'auto_send', value: string) => {
    const next = new URLSearchParams(params)
    if (value === '' || (key === 'state' && value === 'active')) {
      next.delete(key)
    } else {
      next.set(key, value)
    }
    next.delete('page')
    setParams(next, { replace: true })
  }

  return (
    <Page title={t('recurring.title')} actions={<Link className="btn btn--accent" to={`${base}/new`}>{t('recurring.new')}</Link>}>
      <div className="filter-bar" role="search" aria-label={t('recurring.filters')}>
        <label className="filter-bar__field">
          <span>{t('invoices.status')}</span>
          <select value={filter.state} onChange={(e) => change('state', e.target.value)}>
            {['active', 'inactive', 'all'].map((s) => <option key={s} value={s}>{t(`recurring.states.${s}`)}</option>)}
          </select>
        </label>
        <label className="filter-bar__field">
          <span>{t('recurring.frequency')}</span>
          <select value={filter.frequency} onChange={(e) => change('frequency', e.target.value)}>
            <option value="">{t('recurring.allFrequencies')}</option>
            {frequencyTypes.map((f) => <option key={f} value={f}>{t(`recurring.types.${f}`)}</option>)}
          </select>
        </label>
        <label className="filter-bar__field">
          <span>{t('recurring.autoSend')}</span>
          <select value={filter.autoSend} onChange={(e) => change('auto_send', e.target.value)}>
            <option value="">{t('recurring.autoSendAny')}</option>
            <option value="true">{t('recurring.yes')}</option>
            <option value="false">{t('recurring.no')}</option>
          </select>
        </label>
      </div>
      {!invoices.data ? (
        invoices.error ? <p className="empty">{t('recurring.loadError')}</p> : <Loading />
      ) : invoices.data.invoices.length === 0 ? (
        <p className="empty">{isDefault && page === 1 ? t('recurring.empty') : t('recurring.noMatches')}</p>
      ) : (
        <>
          <table className="data-table invoices" aria-label={t('recurring.title')}>
            <thead>
              <tr>
                <th scope="col">{t('invoices.contact')}</th>
                <th scope="col">{t('recurring.frequency')}</th>
                <th scope="col" className="invoices__date">{t('recurring.nextDate')}</th>
                <th scope="col">{t('recurring.autoSend')}</th>
                <th scope="col" className="invoices__amount">{t('invoices.amount')}</th>
              </tr>
            </thead>
            <tbody>
              {invoices.data.invoices.map((r) => (
                <tr key={r.id}>
                  <td>
                    <Link to={`${base}/${r.id}`}>{r.contact ? contactName(r.contact) : r.reference || r.id}</Link>
                    {r.reference && <span className="hint"> · {r.reference}</span>}
                  </td>
                  <td>{frequencyText(r, t)}</td>
                  <td className="invoices__date">{r.active ? date(r.invoice_date) : <span className="badge">{t('recurring.states.inactive')}</span>}</td>
                  <td>{r.auto_send ? t('recurring.yes') : t('recurring.no')}</td>
                  <td className="invoices__amount">{formatAmount(r.total_price_incl_tax, r.currency ?? 'EUR', i18n.language)}</td>
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
