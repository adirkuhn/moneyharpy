import { type DragEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors } from '../forms/Form'
import { filterTerms, type ListFilter, parseFilter, perPage } from '../invoices/queries'
import { StateBadge } from '../invoices/StateBadge'
import { ListIcon, SpinnerIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentFilter, type Term } from '../list/DocumentFilter'
import { AttachmentIcon, DocumentTable, listDate } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import {
  type DocumentSummary, documentPath, documentStates, filterTypes, hasAmount, isReading, typeTags, useDeleteDocuments, useDocuments, useDocumentStatistics, useUploadEach,
} from './documents'

/** The quick filters of Moneybird's Incoming page, as the filters they stand for. */
const quickFilters: Record<string, string | null> = {
  this_year: null,
  all: 'state:all',
  new: 'state:new',
  unpaid: 'state:open|late',
  general: 'type:general_document',
  journals: 'type:general_journal_document',
}

const canonical = (filter: string | null) => filterTerms(parseFilter(filter))

function quickOf(f: ListFilter) {
  const terms = filterTerms(f)
  const found = Object.entries(quickFilters).find(([, q]) => canonical(q) === terms)
  return found?.[0] ?? 'custom'
}

/** The address of the list with a filter; this year's documents need none. */
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

/** The documents a filter selects of every type, as the reference cell shows them: their type and reference. */
const title = (d: DocumentSummary) => `${typeTags[d.type]} ${d.reference}`

const maxSize = 10 * 1024 * 1024

/** Every incoming document, of every type, filtered and paged (/documents), after Moneybird's Incoming page. */
export function DocumentsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = parseFilter(params.get('filter'))
  const terms = filterTerms(filter)
  const page = Math.max(1, Number(params.get('page')) || 1)
  const documents = useDocuments(administrationId, terms, page)
  const administration = useAdministration(administrationId)
  const quick = quickOf(filter)
  const base = `/${administrationId}/documents`
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [result, setResult] = useState('')
  const [info, setInfo] = useState<boolean | null>(null)
  const [statsShown, setStatsShown] = useState(false)
  const introId = useId()
  const statsId = useId()
  const shown = documents.data?.documents ?? []
  const chosen = shown.filter((d) => selected.has(d.id))
  const firstUse = documents.data !== undefined && shown.length === 0 && quick === 'this_year' && page === 1
  const introShown = info ?? firstUse
  const currency = administration.data?.currency ?? 'EUR'

  const apply = (next: ListFilter) => {
    setParams(listQuery(next, params), { replace: true })
    setSelected(new Set())
  }
  const linkTo = (q: string | null) => {
    const query = listQuery(parseFilter(q), new URLSearchParams()).toString()
    return query ? `${base}?${query}` : base
  }
  const options = (values: readonly string[], label: (v: string) => string) => [{ value: '', label: t('list.all') }, ...values.map((v) => ({ value: v, label: label(v) }))]
  const extraTerms: Term[] = [
    { key: 'type', legend: t('incoming.type'), options: options(filterTypes, (v) => t(`incoming.types.${v}`)) },
    { key: 'recurring', legend: t('incoming.recurring'), options: options(['enabled', 'disabled'], (v) => t(`incoming.recurringValues.${v}`)) },
    { key: 'attachment', legend: t('incoming.attachment'), options: options(['with', 'without'], (v) => t(`incoming.attachmentValues.${v}`)) },
    { key: 'automatic_saving', legend: t('incoming.automaticSaving'), options: options(['automatic', 'manual'], (v) => t(`incoming.automaticSavingValues.${v}`)) },
  ]

  const filterPanel = (
    <FilterPanel
      label={t(`incoming.views.${quick}`)}
      title={t('incoming.filters')}
      quick={Object.entries(quickFilters).map(([key, q]) => ({ label: t(`incoming.quick.${key}`), to: linkTo(q), current: key === quick }))}
    >
      <DocumentFilter
        administrationId={administrationId}
        filter={filter}
        states={options(documentStates, (s) => t(`invoices.states.${s}`))}
        terms={extraTerms}
        onApply={apply}
      />
    </FilterPanel>
  )

  return (
    <Page
      wide
      title={t('incoming.title')}
      lead={filterPanel}
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          statistics={{ shown: statsShown, onToggle: () => setStatsShown(!statsShown), controls: statsId }}
          exports={[t('incoming.excelPerDocument'), t('list.excelPerRow'), t('list.composeExport')]}
          primary={{ to: `${base}/new`, label: t('incoming.add') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('incoming.intro.title')}
          text={t('incoming.intro.text')}
          steps={[
            { icon: 'upload', title: t('incoming.intro.upload'), text: t('incoming.intro.uploadText') },
            { icon: 'file', title: t('incoming.intro.check'), text: t('incoming.intro.checkText') },
            { icon: 'coins', title: t('incoming.intro.pay'), text: t('incoming.intro.payText') },
          ]}
        />
      )}
      {statsShown && <Statistics id={statsId} administrationId={administrationId} filter={terms} />}
      <DropBar administrationId={administrationId} />
      {result && chosen.length === 0 && <p className="notice" role="status">{result}</p>}
      {!documents.data ? (
        documents.error ? <p className="empty">{t('incoming.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{firstUse ? t('incoming.empty') : t('incoming.noMatches')}</p>
      ) : (
        <>
          <DocumentTable
            label={t('incoming.title')}
            rows={shown}
            rowKey={(d) => d.id}
            href={(d) => documentPath(administrationId, d)}
            selection={{ selected, onChange: setSelected, selectLabel: (d) => t('incoming.bulk.select', { name: title(d) }), allLabel: t('incoming.bulk.selectAll') }}
            total={documents.data.total}
            perPage={perPage}
            columns={[
              {
                key: 'reference', header: t('incoming.reference'), link: true,
                // The tag stays beside a long reference, such as a file name, which wraps on its own.
                // An upload being read says so until Scan and Recognise has filled it in.
                cell: (d) => isReading(d) ? (
                  <span className="document-reading"><SpinnerIcon /> {t('recognition.recognising')}</span>
                ) : <span className="document-type-ref"><span className="document-type-tag">{typeTags[d.type]}</span> <span>{d.reference}</span></span>,
              },
              { key: 'icons', header: '', label: t('list.attachments'), kind: 'icons', cell: (d) => <AttachmentIcon count={d.attachments_count} /> },
              { key: 'contact', header: t('incoming.contact'), cell: (d) => d.contact_name || <span className="empty">{t('incoming.noContact')}</span> },
              { key: 'date', header: t('incoming.date'), kind: 'date', cell: (d) => listDate(d.date) },
              {
                key: 'amount', header: t('incoming.amount'), kind: 'amount',
                cell: (d) => {
                  const amount = formatAmount(d.total_price_incl_tax, d.currency || currency, i18n.language)
                  return hasAmount(d.type) ? amount : <span className="empty">{amount}</span>
                },
              },
              { key: 'state', header: t('incoming.status'), kind: 'state', cell: (d) => <StateBadge state={d.state} /> },
            ]}
          />
          <BulkActions
            administrationId={administrationId}
            documents={chosen}
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

/** What can be done with the documents ticked at once, in a bar at the bottom of the screen. */
function BulkActions({ administrationId, documents, onDone }: { administrationId: string; documents: DocumentSummary[]; onDone: (message: string) => void }) {
  const { t } = useTranslation()
  const remove = useDeleteDocuments(administrationId)
  if (documents.length === 0) {
    return null
  }
  return (
    <div className="bulk-bar" role="group" aria-label={t('incoming.bulk.label')}>
      <span className="bulk-bar__count" role="status">{t('incoming.bulk.selected', { count: documents.length })}</span>
      <div className="bulk-bar__actions">
        <ConfirmDelete
          label={t('incoming.bulk.delete')}
          confirmLabel={t('incoming.bulk.confirmDelete', { count: documents.length })}
          disabled={remove.isPending}
          onConfirm={() =>
            remove.mutate(documents, {
              onSuccess: (failed) =>
                onDone(failed.length > 0
                  ? t('incoming.bulk.failed', { names: failed.map(title).join(', ') })
                  : t('incoming.bulk.deleted', { count: documents.length })),
            })}
        />
      </div>
    </div>
  )
}

/** The green bar that makes a document of every file dropped on it or chosen. */
function DropBar({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const upload = useUploadEach(administrationId)
  const [over, setOver] = useState(false)
  const [problems, setProblems] = useState<string[]>([])

  const take = (list: FileList | null) => {
    const files = [...(list ?? [])]
    const tooLarge = files.filter((f) => f.size > maxSize)
    const fitting = files.filter((f) => f.size <= maxSize)
    setProblems(tooLarge.length > 0 ? [t('incoming.tooLarge', { names: tooLarge.map((f) => f.name).join(', ') })] : [])
    if (fitting.length === 0) {
      return
    }
    upload.mutate(fitting, {
      onSuccess: ({ refused }) => {
        if (refused.length > 0) {
          setProblems((p) => [...p, t('incoming.refused', { names: refused.join(', ') })])
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
    <div className="drop-bar-wrap">
      <label
        className={`drop-bar${over ? ' is-over' : ''}${upload.isPending ? ' is-busy' : ''}`}
        onDragOver={(e) => { e.preventDefault(); setOver(true) }}
        onDragLeave={() => setOver(false)}
        onDrop={drop}
      >
        <ListIcon kind="upload" />
        <span>{upload.isPending ? t('incoming.uploading') : t('incoming.drop')}</span>
        <input
          className="sr-only"
          type="file"
          multiple
          accept="application/pdf,image/*,text/plain"
          aria-label={t('incoming.drop')}
          disabled={upload.isPending}
          onChange={(e) => {
            take(e.target.files)
            e.target.value = ''
          }}
        />
      </label>
      <Problems messages={[...problems, ...formErrors(upload.error, {}).general]} />
    </div>
  )
}

/** The costs and the amount payable of the selection, and how the costs are distributed, after Moneybird's statistics. */
function Statistics({ id, administrationId, filter }: { id: string; administrationId: string; filter: string }) {
  const { t, i18n } = useTranslation()
  const stats = useDocumentStatistics(administrationId, filter, true)
  const heading = useId()
  const base = `/${administrationId}`
  const money = (amount: string) => formatAmount(amount, stats.data?.currency ?? 'EUR', i18n.language)
  const categories = (stats.data?.categories ?? []).filter((c) => Number(c.amount) > 0)
  const total = categories.reduce((sum, c) => sum + Number(c.amount), 0)

  return (
    <section id={id} className="incoming-stats" aria-labelledby={heading}>
      <h2 id={heading} className="sr-only">{t('incoming.statistics')}</h2>
      {!stats.data ? (
        stats.error ? <p className="empty">{t('incoming.statisticsError')}</p> : <Loading />
      ) : (
        <>
          <div className="incoming-stats__totals">
            <div>
              <h3 className="incoming-stats__total">
                {t('incoming.costs')} <span>{money(stats.data.costs_excl_tax)}</span> <small>{t('incoming.exclTax')}</small>
              </h3>
              <p className="incoming-stats__note">{t('incoming.inSelection')}</p>
              <Link to={`${base}/report/profit_loss`}>{t('incoming.profitLoss')}</Link>
            </div>
            <div>
              <h3 className="incoming-stats__total">
                {t('incoming.payable')} <span>{money(stats.data.amount_payable)}</span> <small>{t('incoming.inclTax')}</small>
              </h3>
              <p className="incoming-stats__note">{t('incoming.inSelection')}</p>
              <Link to={`${base}/report/creditors`}>{t('incoming.agedPayables')}</Link>
            </div>
          </div>
          <div className="incoming-stats__distribution">
            <h3 className="incoming-stats__title">{t('incoming.distribution')}</h3>
            <p className="incoming-stats__note">{t('incoming.distributionText')}</p>
            {categories.length === 0 ? (
              <p className="empty">{t('incoming.noCosts')}</p>
            ) : (
              <>
                <div className="incoming-stats__bar" aria-hidden="true">
                  {categories.map((c, i) => (
                    <span key={`${c.ledger_account_id}-${c.name}`} className={`incoming-stats__part incoming-stats__color-${i % 6}`} style={{ flexGrow: Number(c.amount) / total }} />
                  ))}
                </div>
                <ul className="incoming-stats__legend">
                  {categories.map((c, i) => (
                    <li key={`${c.ledger_account_id}-${c.name}`}>
                      <span className={`incoming-stats__marker incoming-stats__color-${i % 6}`} aria-hidden="true" />
                      <span className="incoming-stats__amount">{money(c.amount)}</span>
                      <span className="incoming-stats__name">{c.name}</span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </div>
        </>
      )}
    </section>
  )
}
