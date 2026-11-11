import { type DragEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { formErrors } from '../forms/Form'
import { filterTerms, type ListFilter, parseFilter, perPage } from '../invoices/queries'
import { StateBadge } from '../invoices/StateBadge'
import { reportPeriods } from '../journal/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Pagination } from '../layout/Pagination'
import { externalStates, externalTitle, unpaidExternalStates, useExternalInvoices, useUploadExternalInvoices } from './queries'

const maxSize = 10 * 1024 * 1024

/** Invoices made in other software, filtered by state and period, with an upload area for their files (/external_sales_invoices). */
export function ExternalInvoicesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = parseFilter(params.get('filter'))
  const page = Math.max(1, Number(params.get('page')) || 1)
  const invoices = useExternalInvoices(administrationId, filterTerms(filter), page)
  const administration = useAdministration(administrationId)
  const base = `/${administrationId}/external_sales_invoices`
  const date = (d: string | null | undefined) => (d ? new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' }) : '')
  const isDefault = !filter.state && filter.period === 'this_year' && filter.rest.length === 0

  const change = (changes: Partial<ListFilter>) => {
    const nextFilter = { ...filter, ...changes }
    const next = new URLSearchParams(params)
    if (!nextFilter.state && nextFilter.period === 'this_year' && nextFilter.rest.length === 0) {
      next.delete('filter')
    } else {
      next.set('filter', filterTerms(nextFilter))
    }
    next.delete('page')
    setParams(next, { replace: true })
  }

  return (
    <Page title={t('external.title')} actions={<Link className="btn btn--accent" to={`${base}/new`}>{t('external.new')}</Link>}>
      <UploadArea administrationId={administrationId} />
      <div className="filter-bar" role="search" aria-label={t('external.filters')}>
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
            <option value={unpaidExternalStates}>{t('invoices.quick.unpaid')}</option>
            {externalStates.map((s) => <option key={s} value={s}>{t(`invoices.states.${s}`)}</option>)}
          </select>
        </label>
      </div>
      {!invoices.data ? (
        invoices.error ? <p className="empty">{t('external.loadError')}</p> : <Loading />
      ) : invoices.data.invoices.length === 0 ? (
        <p className="empty">{isDefault && page === 1 ? t('external.empty') : t('invoices.noMatches')}</p>
      ) : (
        <>
          <table className="data-table invoices" aria-label={t('external.title')}>
            <thead>
              <tr>
                <th scope="col" className="invoices__date">{t('invoices.invoiceDate')}</th>
                <th scope="col">{t('invoices.contact')}</th>
                <th scope="col" className="invoices__number">{t('invoices.reference')}</th>
                <th scope="col" className="invoices__amount">{t('invoices.amount')}</th>
                <th scope="col" className="invoices__state">{t('invoices.status')}</th>
              </tr>
            </thead>
            <tbody>
              {invoices.data.invoices.map((inv) => (
                <tr key={inv.id}>
                  <td className="invoices__date">{date(inv.date)}</td>
                  <td>{inv.contact ? contactName(inv.contact) : ''}</td>
                  <td className="invoices__number"><Link to={`${base}/${inv.id}`}>{externalTitle(inv, t)}</Link></td>
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

/** Takes the files of invoices, dropped or chosen, and makes a new invoice of each; the first opens. */
function UploadArea({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const id = useId()
  const navigate = useNavigate()
  const upload = useUploadExternalInvoices(administrationId)
  const [over, setOver] = useState(false)
  const [problems, setProblems] = useState<string[]>([])

  const take = (list: FileList | null) => {
    const files = [...(list ?? [])]
    const tooLarge = files.filter((f) => f.size > maxSize)
    const fitting = files.filter((f) => f.size <= maxSize)
    setProblems(tooLarge.length > 0 ? [t('external.tooLarge', { names: tooLarge.map((f) => f.name).join(', ') })] : [])
    if (fitting.length === 0) {
      return
    }
    upload.mutate(fitting, {
      onSuccess: ({ made, refused }) => {
        if (refused.length > 0) {
          setProblems((p) => [...p, t('external.refused', { names: refused.join(', ') })])
        }
        if (made[0]?.id) {
          navigate(`/${administrationId}/external_sales_invoices/${made[0].id}`)
        }
      },
    })
  }
  const drop = (e: DragEvent) => {
    e.preventDefault()
    setOver(false)
    take(e.dataTransfer.files)
  }

  return (
    <section
      className={`upload-area${over ? ' is-over' : ''}`}
      aria-labelledby={`${id}-title`}
      onDragOver={(e) => { e.preventDefault(); setOver(true) }}
      onDragLeave={() => setOver(false)}
      onDrop={drop}
    >
      <h2 id={`${id}-title`} className="upload-area__title">{t('external.uploadTitle')}</h2>
      <p className="hint">{upload.isPending ? t('invoices.uploading') : t('external.uploadHelp')}</p>
      <Problems messages={[...problems, ...formErrors(upload.error, {}).general]} />
      <label className="btn btn--secondary btn--small file-field__pick" htmlFor={`${id}-file`}>{t('external.choose')}</label>
      <input
        id={`${id}-file`}
        className="sr-only"
        type="file"
        multiple
        accept="application/pdf,image/*,text/plain"
        aria-label={t('external.upload')}
        disabled={upload.isPending}
        onChange={(e) => {
          take(e.target.files)
          e.target.value = ''
        }}
      />
    </section>
  )
}
