import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { ContactName } from '../contacts/ContactHoverCard'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { perPage } from '../invoices/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { AdvancedFilter, ContactFilter, FilterPanel, RadioFilter } from '../list/FilterPanel'
import { ListActions } from '../list/ListActions'
import {
  type Estimate, type EstimateFilter, estimateFilterOf, estimateFilterTerms, estimateQuery, estimateStates, estimateTitle, useEstimates,
} from './queries'

/** The estimates (quotes), newest first, filtered by state and contact (/estimates). */
export function EstimatesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = estimateFilterOf(params)
  const page = Math.max(1, Number(params.get('page')) || 1)
  const estimates = useEstimates(administrationId, estimateFilterTerms(filter), page)
  const base = `/${administrationId}/estimates`
  const view = filter.contactId ? 'custom' : filter.state || 'all'
  const shown = estimates.data?.estimates ?? []
  const firstUse = estimates.data !== undefined && shown.length === 0 && view === 'all' && page === 1

  return (
    <Page
      wide
      title={t('estimates.title')}
      lead={
        <FilterPanel
          label={t(`estimates.views.${view}`)}
          title={t('estimates.filters')}
          quick={['all', 'draft', 'open', 'accepted'].map((s) => ({
            label: t(`estimates.views.${s}`),
            to: s === 'all' ? base : `${base}?state=${s}`,
            current: view === s,
          }))}
        >
          <EstimateFilterForm administrationId={administrationId} filter={filter} reset={base} onApply={(next) => setParams(estimateQuery(next, params), { replace: true })} />
        </FilterPanel>
      }
      actions={<ListActions exports={[]} primary={{ to: `${base}/new`, label: t('estimates.add') }} />}
    >
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
            { key: 'contact', header: t('invoices.contact'), link: true, cell: (e) => (e.contact ? <ContactName administrationId={administrationId} contactId={e.contact_id} name={contactName(e.contact)} /> : estimateTitle(e, t)) },
            { key: 'reference', header: t('invoices.reference'), kind: 'extra', cell: (e) => e.reference },
            { key: 'number', header: t('estimates.number'), cell: (e) => estimateTitle(e, t) },
            { key: 'date', header: t('estimates.date'), kind: 'date', cell: (e) => listDate(e.estimate_date) },
            { key: 'amount', header: t('invoices.amount'), kind: 'amount', cell: (e) => formatAmount(e.total_price_incl_tax, e.currency ?? 'EUR', i18n.language) },
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

type FormProps = { administrationId: string; filter: EstimateFilter; reset: string; onApply: (filter: EstimateFilter) => void }

function EstimateFilterForm({ administrationId, filter, reset, onApply }: FormProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(filter)
  return (
    <AdvancedFilter value={draft} onApply={() => onApply(draft)} footer={<Link className="btn btn--secondary" to={reset}>{t('estimates.showAll')}</Link>}>
      <RadioFilter
        legend={t('estimates.status')}
        value={draft.state}
        onChange={(state) => setDraft({ ...draft, state })}
        options={[{ value: '', label: t('list.all') }, ...estimateStates.map((s) => ({ value: s, label: t(`estimates.states.${s}`) }))]}
      />
      <ContactFilter administrationId={administrationId} contactId={draft.contactId} onChange={(contactId) => setDraft({ ...draft, contactId })} />
    </AdvancedFilter>
  )
}
