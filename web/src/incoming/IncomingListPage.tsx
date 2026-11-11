import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { filterTerms, type ListFilter, parseFilter, perPage } from '../invoices/queries'
import { StateBadge } from '../invoices/StateBadge'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentFilter } from '../list/DocumentFilter'
import { AttachmentIcon, DocumentTable, listDate } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { ListActions } from '../list/ListActions'
import { type IncomingKind, incomingStates, incomingTitle, unpaidStates, useIncomingDocuments } from './queries'

/** The quick filters of Moneybird's incoming document lists. */
const quickFilters: Record<string, Pick<ListFilter, 'state' | 'period'>> = {
  this_year: { state: '', period: 'this_year' },
  unpaid: { state: unpaidStates, period: '' },
  paid: { state: 'paid', period: '' },
}

function quickOf(f: ListFilter) {
  const found = Object.entries(quickFilters).find(([, q]) => q.state === f.state && q.period === f.period && f.rest.length === 0)
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

/** Purchase invoices or receipts, filtered by state, period and contact (/documents/purchase_invoices, /documents/receipts). */
export function IncomingListPage({ kind }: { kind: IncomingKind }) {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = parseFilter(params.get('filter'))
  const page = Math.max(1, Number(params.get('page')) || 1)
  const documents = useIncomingDocuments(administrationId, kind, filterTerms(filter), page)
  const administration = useAdministration(administrationId)
  const quick = quickOf(filter)
  const base = `/${administrationId}/documents/${kind}`
  const shown = documents.data?.documents ?? []
  const linkTo = (f: Pick<ListFilter, 'state' | 'period'>) => {
    const query = listQuery({ ...f, rest: [] }, new URLSearchParams()).toString()
    return query ? `${base}?${query}` : base
  }
  const title = t(`purchases.titles.${kind}`)

  const filterPanel = (
    <FilterPanel
      label={t(`purchases.views.${quick}`)}
      title={t('purchases.filters')}
      quick={Object.entries(quickFilters).map(([key, f]) => ({ label: t(`purchases.views.${key}`), to: linkTo(f), current: key === quick }))}
    >
      <DocumentFilter
        administrationId={administrationId}
        filter={filter}
        states={[{ value: '', label: t('list.all') }, ...incomingStates.map((s) => ({ value: s, label: t(`invoices.states.${s}`) }))]}
        onApply={(next) => setParams(listQuery(next, params), { replace: true })}
      />
    </FilterPanel>
  )

  return (
    <Page
      wide
      title={title}
      lead={filterPanel}
      actions={<ListActions exports={[]} primary={{ to: `/${administrationId}/documents/typeless_documents`, label: t('purchases.upload') }} />}
    >
      {!documents.data ? (
        documents.error ? <p className="empty">{t('purchases.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{t('purchases.empty')}</p>
      ) : (
        <DocumentTable
          label={title}
          rows={shown}
          rowKey={(d) => d.id ?? ''}
          href={(d) => `${base}/${d.id}`}
          total={documents.data.total}
          perPage={perPage}
          columns={[
            { key: 'reference', header: t('invoices.reference'), link: true, cell: (d) => incomingTitle(d, kind, t) },
            { key: 'icons', header: '', label: t('list.attachments'), kind: 'icons', cell: (d) => <AttachmentIcon count={d.attachments?.length ?? 0} /> },
            { key: 'contact', header: t('invoices.contact'), cell: (d) => (d.contact ? contactName(d.contact) : '') },
            { key: 'date', header: t('purchases.date'), kind: 'date', cell: (d) => listDate(d.date) },
            {
              key: 'amount', header: t('invoices.amount'), kind: 'amount',
              cell: (d) => formatAmount(d.total_price_incl_tax ?? '0', d.currency ?? administration.data?.currency ?? 'EUR', i18n.language),
            },
            { key: 'state', header: t('invoices.status'), kind: 'state', cell: (d) => <StateBadge state={d.state} /> },
          ]}
        />
      )}
    </Page>
  )
}
