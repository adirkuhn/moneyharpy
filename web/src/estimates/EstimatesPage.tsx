import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { ContactHoverCard } from '../contacts/ContactHoverCard'
import { filterTerms, type ListFilter, parseFilter, perPage } from '../invoices/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentFilter } from '../list/DocumentFilter'
import { AttachmentIcon, DocumentTable, listDate } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import { type Estimate, estimateStates, estimateTitle, quickFilters, useEstimates } from './queries'

function quickOf(f: ListFilter) {
  const found = Object.entries(quickFilters).find(([, q]) => q.state === f.state && q.period === f.period && f.rest.length === 0)
  return found?.[0] ?? 'custom'
}

/** The address of the list with a filter; this year's quotes need none. */
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

/**
 * The quotes after Moneybird's list (/estimates): this year's by default,
 * with its quick filters and the advanced filter on period, state, contact
 * and workflow, newest first and paged.
 */
export function EstimatesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = parseFilter(params.get('filter'))
  const page = Math.max(1, Number(params.get('page')) || 1)
  const estimates = useEstimates(administrationId, filterTerms(filter), page)
  const administration = useAdministration(administrationId)
  const quick = quickOf(filter)
  const base = `/${administrationId}/estimates`
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const shown = estimates.data?.estimates ?? []
  const firstUse = estimates.data !== undefined && shown.length === 0 && quick === 'this_year' && page === 1
  const introShown = info ?? firstUse

  const linkTo = (f: Pick<ListFilter, 'state' | 'period'>) => {
    const query = listQuery({ ...f, rest: [] }, new URLSearchParams()).toString()
    return query ? `${base}?${query}` : base
  }

  const filterPanel = (
    <FilterPanel
      label={t(`estimates.views.${quick}`)}
      title={t('estimates.filters')}
      quick={Object.entries(quickFilters).map(([key, f]) => ({ label: t(`estimates.quick.${key}`), to: linkTo(f), current: key === quick }))}
    >
      <DocumentFilter
        administrationId={administrationId}
        filter={filter}
        workflows="EstimateWorkflow"
        states={[{ value: '', label: t('list.all') }, ...estimateStates.map((s) => ({ value: s, label: t(`estimates.states.${s}`) }))]}
        onApply={(next) => setParams(listQuery(next, params), { replace: true })}
      />
    </FilterPanel>
  )

  return (
    <Page
      wide
      title={t('estimates.title')}
      lead={filterPanel}
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          exports={[t('estimates.exports.perQuote'), t('list.excelPerRow'), t('list.composeExport')]}
          primary={{ to: `${base}/new`, label: t('estimates.add') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('estimates.intro.title')}
          text={t('estimates.intro.text')}
          steps={[
            { icon: 'palette', title: t('estimates.intro.layout'), text: t('estimates.intro.layoutText') },
            { icon: 'edit', title: t('estimates.intro.sign'), text: t('estimates.intro.signText') },
            { icon: 'file', title: t('estimates.intro.bill'), text: t('estimates.intro.billText') },
          ]}
        />
      )}
      {!estimates.data ? (
        estimates.error ? <p className="empty">{t('estimates.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{firstUse ? t('estimates.empty') : t('estimates.noMatches')}</p>
      ) : (
        <DocumentTable<Estimate>
          label={t('estimates.title')}
          rows={shown}
          rowKey={(e) => e.id ?? ''}
          href={(e) => `${base}/${e.id}`}
          total={estimates.data.total}
          perPage={perPage}
          columns={[
            { key: 'number', header: t('estimates.quoteId'), link: true, cell: (e) => (e.estimate_id ? e.estimate_id : <em className="draft-id">{estimateTitle(e, t)}</em>) },
            { key: 'reference', header: t('invoices.reference'), kind: 'extra', cell: (e) => e.reference },
            { key: 'icons', header: '', label: t('list.attachments'), kind: 'icons', cell: (e) => <AttachmentIcon count={e.attachments?.length ?? 0} /> },
            { key: 'date', header: t('estimates.date'), kind: 'date', cell: (e) => listDate(e.estimate_date) },
            { key: 'contact', header: t('invoices.contact'), cell: (e) => (e.contact ? <ContactHoverCard administrationId={administrationId} contact={{ ...e.contact, id: e.contact_id }} /> : '') },
            {
              key: 'amount', header: t('invoices.amount'), kind: 'amount',
              cell: (e) => formatAmount(e.total_price_incl_tax, e.currency ?? administration.data?.currency ?? 'EUR', i18n.language),
            },
            { key: 'state', header: t('estimates.status'), kind: 'state', cell: (e) => <EstimateBadge state={e.state} /> },
          ]}
        />
      )}
    </Page>
  )
}

/** The state of an estimate as a coloured label. */
export function EstimateBadge({ state }: { state: string | undefined }) {
  const { t } = useTranslation()
  const s = state ?? 'draft'
  return <span className={`badge badge--${s}`}>{t(`estimates.states.${s}`)}</span>
}
