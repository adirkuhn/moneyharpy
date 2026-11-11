import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { perPage } from '../invoices/queries'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { WorkflowFilter } from '../list/DocumentFilter'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { AdvancedFilter, ContactFilter, FilterPanel, RadioFilter } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import {
  frequencyText, frequencyTypes, type RecurringFilter, recurringFilterOf, recurringFilterTerms, recurringQuery, type RecurringInvoice, useRecurringInvoices,
} from './queries'

/** Recurring invoices, active ones unless filtered otherwise (/recurring_sales_invoices). */
export function RecurringInvoicesPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = recurringFilterOf(params)
  const page = Math.max(1, Number(params.get('page')) || 1)
  const invoices = useRecurringInvoices(administrationId, recurringFilterTerms(filter), page)
  const base = `/${administrationId}/recurring_sales_invoices`
  const narrowed = !!(filter.frequency || filter.autoSend || filter.contactId || filter.workflowId)
  const view = narrowed || !['active', 'inactive', 'all'].includes(filter.state) ? 'custom' : filter.state
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const shown = invoices.data?.invoices ?? []
  const firstUse = invoices.data !== undefined && shown.length === 0 && view === 'active' && page === 1
  const introShown = info ?? firstUse

  return (
    <Page
      wide
      title={t('recurring.title')}
      lead={
        <FilterPanel label={t(`recurring.views.${view}`)} title={t('recurring.filters')}>
          <RecurringFilterForm
            administrationId={administrationId}
            filter={filter}
            reset={base}
            onApply={(next) => setParams(recurringQuery(next, params), { replace: true })}
          />
        </FilterPanel>
      }
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          exports={[]}
          primary={{ to: `${base}/new`, label: t('recurring.add') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('recurring.intro.title')}
          text={t('recurring.intro.text')}
          steps={[
            { icon: 'calendar', title: t('recurring.intro.start'), text: t('recurring.intro.startText') },
            { icon: 'repeat', title: t('recurring.intro.frequency'), text: t('recurring.intro.frequencyText') },
            { icon: 'send', title: t('recurring.intro.send'), text: t('recurring.intro.sendText') },
          ]}
        />
      )}
      {!invoices.data ? (
        invoices.error ? <p className="empty">{t('recurring.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{firstUse ? t('recurring.empty') : t('recurring.noMatches')}</p>
      ) : (
        <DocumentTable<RecurringInvoice>
          label={t('recurring.title')}
          rows={shown}
          rowKey={(r) => r.id ?? ''}
          href={(r) => `${base}/${r.id}`}
          total={invoices.data.total}
          perPage={perPage}
          columns={[
            { key: 'contact', header: t('invoices.contact'), link: true, cell: (r) => (r.contact ? contactName(r.contact) : r.reference || r.id) },
            { key: 'reference', header: t('invoices.reference'), kind: 'extra', cell: (r) => r.reference },
            { key: 'icons', header: '', label: t('recurring.autoSend'), kind: 'icons', cell: (r) => r.auto_send && <AutoSendIcon /> },
            { key: 'frequency', header: t('recurring.frequency'), cell: (r) => frequencyText(r, t) },
            { key: 'next', header: t('recurring.nextInvoiceDate'), kind: 'date', cell: (r) => (r.active ? listDate(r.invoice_date) : '') },
            { key: 'amount', header: t('invoices.amount'), kind: 'amount', cell: (r) => formatAmount(r.total_price_incl_tax, r.currency ?? 'EUR', i18n.language) },
            {
              key: 'state', header: t('recurring.status'), kind: 'state',
              cell: (r) => <span className={`badge badge--${r.active ? 'active' : 'inactive'}`}>{t(`recurring.states.${r.active ? 'active' : 'inactive'}`)}</span>,
            },
          ]}
        />
      )}
    </Page>
  )
}

function AutoSendIcon() {
  const { t } = useTranslation()
  return (
    <span className="document-table__icon" role="img" aria-label={t('recurring.autoSend')} title={t('recurring.autoSend')}>
      <ListIcon kind="send" />
    </span>
  )
}

type FormProps = { administrationId: string; filter: RecurringFilter; reset: string; onApply: (filter: RecurringFilter) => void }

/** Moneybird's filter of recurring invoices: status, frequency, sending, contact and workflow. */
function RecurringFilterForm({ administrationId, filter, reset, onApply }: FormProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(filter)
  const set = (changes: Partial<RecurringFilter>) => setDraft({ ...draft, ...changes })
  return (
    <AdvancedFilter value={draft} onApply={() => onApply(draft)} footer={<Link className="btn btn--secondary" to={reset}>{t('recurring.showAllActive')}</Link>}>
      <RadioFilter
        legend={t('recurring.status')}
        value={draft.state}
        onChange={(state) => set({ state })}
        options={['active', 'inactive', 'all'].map((s) => ({ value: s, label: t(`recurring.filterStates.${s}`) }))}
      />
      <RadioFilter
        legend={t('recurring.frequency')}
        value={draft.frequency}
        onChange={(frequency) => set({ frequency })}
        options={[{ value: '', label: t('list.all') }, ...frequencyTypes.map((f) => ({ value: f, label: t(`recurring.types.${f}`) }))]}
      />
      <RadioFilter
        legend={t('recurring.sendAutomatically')}
        value={draft.autoSend}
        onChange={(autoSend) => set({ autoSend })}
        options={[{ value: '', label: t('list.all') }, { value: 'true', label: t('recurring.yes') }, { value: 'false', label: t('recurring.no') }]}
      />
      <ContactFilter administrationId={administrationId} contactId={draft.contactId} onChange={(contactId) => set({ contactId })} />
      <WorkflowFilter administrationId={administrationId} value={draft.workflowId} onChange={(workflowId) => set({ workflowId })} />
    </AdvancedFilter>
  )
}
