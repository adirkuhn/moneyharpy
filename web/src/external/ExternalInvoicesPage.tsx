import { type DragEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors } from '../forms/Form'
import { filterTerms, type ListFilter, parseFilter, perPage } from '../invoices/queries'
import { StateBadge } from '../invoices/StateBadge'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentFilter } from '../list/DocumentFilter'
import { AttachmentIcon, DocumentTable, listDate } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import {
  type ExternalInvoice, externalListName, externalStates, externalTitle, unpaidExternalStates, useDeleteExternalInvoices, useExternalInvoices, useUploadExternalInvoices,
} from './queries'
import './external.css'

const maxSize = 10 * 1024 * 1024

/** The quick filters of Moneybird's external invoice list. */
const quickFilters: Record<string, Pick<ListFilter, 'state' | 'period'>> = {
  this_year: { state: '', period: 'this_year' },
  unpaid: { state: unpaidExternalStates, period: '' },
  paid: { state: 'paid', period: '' },
}

function quickOf(f: ListFilter) {
  const found = Object.entries(quickFilters).find(([, q]) => q.state === f.state && q.period === f.period && f.rest.length === 0)
  return found?.[0] ?? 'custom'
}

/** The address of the list with a filter; this year's invoices need none. */
function listQuery(f: ListFilter, params: URLSearchParams) {
  const next = new URLSearchParams(params)
  if (quickOf(f) === 'this_year') {
    next.delete('filter')
  } else {
    next.set('filter', filterTerms(f))
  }
  next.delete('page')
  return next
}

/** Invoices made in other software, filtered by state, period and contact, with an upload area for their files (/external_sales_invoices). */
export function ExternalInvoicesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = parseFilter(params.get('filter'))
  const page = Math.max(1, Number(params.get('page')) || 1)
  const invoices = useExternalInvoices(administrationId, filterTerms(filter), page)
  const administration = useAdministration(administrationId)
  const quick = quickOf(filter)
  const base = `/${administrationId}/external_sales_invoices`
  const [info, setInfo] = useState<boolean | null>(null)
  const [emailShown, setEmailShown] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [result, setResult] = useState<string | null>(null)
  const introId = useId()
  const emailId = useId()
  const shown = invoices.data?.invoices ?? []
  const chosen = shown.filter((inv) => selected.has(inv.id ?? ''))
  const firstUse = invoices.data !== undefined && shown.length === 0 && quick === 'this_year' && page === 1
  const introShown = info ?? firstUse
  const linkTo = (f: Pick<ListFilter, 'state' | 'period'>) => {
    const query = listQuery({ ...f, rest: [] }, new URLSearchParams()).toString()
    return query ? `${base}?${query}` : base
  }

  const filterPanel = (
    <FilterPanel
      label={t(`external.views.${quick}`)}
      title={t('external.filters')}
      quick={Object.entries(quickFilters).map(([key, f]) => ({ label: t(`external.quick.${key}`), to: linkTo(f), current: key === quick }))}
    >
      <DocumentFilter
        administrationId={administrationId}
        filter={filter}
        states={[{ value: '', label: t('list.all') }, ...externalStates.map((s) => ({ value: s, label: t(`invoices.states.${s}`) }))]}
        onApply={(next) => setParams(listQuery(next, params), { replace: true })}
      />
    </FilterPanel>
  )

  return (
    <Page
      wide
      title={t('external.title')}
      lead={filterPanel}
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          extra={
            <button
              type="button"
              className={`btn btn--secondary btn--icon${emailShown ? ' is-active' : ''}`}
              aria-label={t('external.emailFiles')}
              title={t('external.emailFiles')}
              aria-expanded={emailShown}
              aria-controls={emailId}
              onClick={() => setEmailShown(!emailShown)}
            >
              <ListIcon kind="at" />
            </button>
          }
          exports={[t('list.excelPerInvoice'), t('list.excelPerRow'), t('list.composeExport')]}
          primary={{ to: `${base}/new`, label: t('external.add') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('external.intro.title')}
          text={t('external.intro.text')}
          steps={[
            { icon: 'coins', title: t('external.intro.outside'), text: t('external.intro.outsideText') },
            { icon: 'upload', title: t('external.intro.add'), text: t('external.intro.addText') },
            { icon: 'plug', title: t('external.intro.api'), text: t('external.intro.apiText') },
          ]}
        />
      )}
      {emailShown && (
        <section id={emailId} className="panel-box" aria-labelledby={`${emailId}-title`}>
          <h2 id={`${emailId}-title`} className="panel-box__title">{t('external.emailTitle')}</h2>
          <p className="panel-box__text">{t('external.emailText')}</p>
          <p className="hint">{t('list.notAvailable')}</p>
          <button type="button" className="btn btn--secondary btn--small" onClick={() => setEmailShown(false)}>{t('external.close')}</button>
        </section>
      )}
      <UploadArea administrationId={administrationId} />
      {result && chosen.length === 0 && <p className="notice" role="status">{result}</p>}
      {!invoices.data ? (
        invoices.error ? <p className="empty">{t('external.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{firstUse ? t('external.empty') : t('external.noMatches')}</p>
      ) : (
        <>
        <DocumentTable
          label={t('external.title')}
          rows={shown}
          rowKey={(inv) => inv.id ?? ''}
          href={(inv) => `${base}/${inv.id}`}
          selection={{ selected, onChange: setSelected, selectLabel: (inv) => t('external.bulk.select', { name: externalListName(inv, t) }), allLabel: t('external.bulk.selectAll') }}
          total={invoices.data.total}
          perPage={perPage}
          columns={[
            {
              key: 'reference', header: t('invoices.reference'), link: true,
              // Moneybird's "Example POS › POS-1001": the software the invoice comes from, then its reference.
              cell: (inv) => (inv.source
                ? <span className="external-reference">{inv.source} <span className="external-reference__arrow">›</span> {externalTitle(inv, t)}</span>
                : externalTitle(inv, t)),
            },
            { key: 'icons', header: '', label: t('list.attachments'), kind: 'icons', cell: (inv) => <AttachmentIcon count={inv.attachments?.length ?? 0} /> },
            { key: 'contact', header: t('invoices.contact'), cell: (inv) => (inv.contact ? contactName(inv.contact) : '') },
            { key: 'date', header: t('external.date'), kind: 'date', cell: (inv) => listDate(inv.date) },
            {
              key: 'amount', header: t('invoices.amount'), kind: 'amount',
              cell: (inv) => formatAmount(inv.total_price_incl_tax, inv.currency ?? administration.data?.currency ?? 'EUR', i18n.language),
            },
            { key: 'state', header: t('invoices.status'), kind: 'state', cell: (inv) => <StateBadge state={inv.state} /> },
          ]}
        />
        <BulkActions
          administrationId={administrationId}
          invoices={chosen}
          onDone={(message) => {
            setResult(message)
            setSelected(new Set())
          }}
        />
        </>
      )}
    </Page>
  )
}

/** What can be done with the invoices ticked in the list at once, in a bar at the bottom of the screen. */
function BulkActions({ administrationId, invoices, onDone }: { administrationId: string; invoices: ExternalInvoice[]; onDone: (message: string) => void }) {
  const { t } = useTranslation()
  const remove = useDeleteExternalInvoices(administrationId)
  if (invoices.length === 0) {
    return null
  }
  return (
    <div className="bulk-bar" role="group" aria-label={t('external.bulk.label')}>
      <span className="bulk-bar__count" role="status">{t('external.bulk.selected', { count: invoices.length })}</span>
      <div className="bulk-bar__actions">
        <ConfirmDelete
          label={t('external.bulk.delete', { count: invoices.length })}
          confirmLabel={t('external.bulk.confirmDelete', { count: invoices.length })}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(invoices, {
            onSuccess: (failed) => onDone(failed.length > 0
              ? t('external.bulk.failed', { names: failed.map((inv) => externalTitle(inv, t)).join(', ') })
              : t('external.bulk.deleted', { count: invoices.length })),
          })}
        />
      </div>
    </div>
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
      className={`upload-area upload-area--bar${over ? ' is-over' : ''}`}
      aria-labelledby={`${id}-title`}
      onDragOver={(e) => { e.preventDefault(); setOver(true) }}
      onDragLeave={() => setOver(false)}
      onDrop={drop}
    >
      <h2 id={`${id}-title`} className="sr-only">{t('external.uploadTitle')}</h2>
      <p className="upload-area__text">
        <ListIcon kind="paperclip" />
        <span>
          {t('external.dropText')}{' '}
          <label className="upload-area__pick" htmlFor={`${id}-file`}>{t('external.select')}</label>{' '}
          <span className="upload-area__limit">{t('external.maxSize')}</span>
        </span>
      </p>
      {upload.isPending && <p className="hint" role="status">{t('invoices.uploading')}</p>}
      <Problems messages={[...problems, ...formErrors(upload.error, {}).general]} />
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
