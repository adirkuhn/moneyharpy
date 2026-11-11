import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors } from '../forms/Form'
import { Problems } from '../auth/AuthLayout'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentFilter } from '../list/DocumentFilter'
import { AttachmentIcon, DocumentTable, listDate } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
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

/** The sales invoices, filtered by state, period, contact and workflow, and paged (/sales_invoices). */
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
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [result, setResult] = useState('')
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const shown = invoices.data?.invoices ?? []
  const chosen = shown.filter((inv) => selected.has(inv.id ?? ''))
  const firstUse = invoices.data !== undefined && shown.length === 0 && quick === 'this_year' && page === 1
  const introShown = info ?? firstUse

  const apply = (next: ListFilter) => {
    setParams(listQuery(next, params), { replace: true })
    setSelected(new Set())
  }
  const linkTo = (f: Pick<ListFilter, 'state' | 'period'>) => {
    const query = listQuery({ ...f, rest: [] }, new URLSearchParams()).toString()
    return query ? `${base}?${query}` : base
  }

  const filterPanel = (
    <FilterPanel
      label={t(`invoices.views.${quick}`)}
      title={t('invoices.filters')}
      quick={Object.entries(quickFilters).map(([key, f]) => ({ label: t(`invoices.quick.${key}`), to: linkTo(f), current: key === quick }))}
    >
      <DocumentFilter
        administrationId={administrationId}
        filter={filter}
        workflows
        states={[
          { value: '', label: t('list.all') },
          ...states.map((s) => ({ value: s, label: t(`invoices.states.${s}`) })),
          { value: unpaidStates, label: t('invoices.unpaidState') },
        ]}
        onApply={apply}
      />
    </FilterPanel>
  )

  return (
    <Page
      wide
      title={t('invoices.title')}
      lead={filterPanel}
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          exports={[t('list.excelPerInvoice'), t('list.excelPerRow'), t('list.composeExport')]}
          primary={{ to: `${base}/new`, label: t('invoices.new') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('invoices.intro.title')}
          text={t('invoices.intro.text')}
          steps={[
            { icon: 'file', title: t('invoices.intro.create'), text: t('invoices.intro.createText') },
            { icon: 'palette', title: t('invoices.intro.style'), text: t('invoices.intro.styleText') },
            { icon: 'flow', title: t('invoices.intro.workflow'), text: t('invoices.intro.workflowText') },
          ]}
        />
      )}
      {result && chosen.length === 0 && <p className="notice" role="status">{result}</p>}
      {!invoices.data ? (
        invoices.error ? <p className="empty">{t('invoices.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{firstUse ? t('invoices.empty') : t('invoices.noMatches')}</p>
      ) : (
        <>
          <DocumentTable
            label={t('invoices.title')}
            rows={shown}
            rowKey={(inv) => inv.id ?? ''}
            href={(inv) => `${base}/${inv.id}`}
            selection={{ selected, onChange: setSelected, selectLabel: (inv) => t('invoices.bulk.select', { name: invoiceTitle(inv, t) }), allLabel: t('invoices.bulk.selectAll') }}
            total={invoices.data.total}
            perPage={perPage}
            columns={[
              { key: 'number', header: t('invoices.invoiceId'), link: true, cell: (inv) => (inv.invoice_id ? inv.invoice_id : <em className="draft-id">{invoiceTitle(inv, t)}</em>) },
              { key: 'reference', header: t('invoices.reference'), kind: 'extra', cell: (inv) => inv.reference },
              { key: 'icons', header: '', label: t('list.attachments'), kind: 'icons', cell: (inv) => <AttachmentIcon count={inv.attachments?.length ?? 0} /> },
              { key: 'date', header: t('invoices.invoiceDate'), kind: 'date', cell: (inv) => listDate(inv.invoice_date) },
              { key: 'contact', header: t('invoices.contact'), cell: (inv) => (inv.contact ? contactName(inv.contact) : '') },
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
function BulkActions({ administrationId, invoices, onDone }: { administrationId: string; invoices: SalesInvoice[]; onDone: (message: string) => void }) {
  const { t } = useTranslation()
  const pay = useRegisterFullPayments(administrationId)
  const remind = useSendReminders(administrationId)
  const remove = useDeleteDrafts(administrationId)
  const payable = invoices.filter((inv) => isPayable(inv.state))
  const drafts = invoices.filter((inv) => inv.state === 'draft')
  const busy = pay.isPending || remind.isPending || remove.isPending
  const report = (done: number, failed: SalesInvoice[], key: string) =>
    onDone(failed.length > 0 ? t('invoices.bulk.failed', { names: failed.map((inv) => invoiceTitle(inv, t)).join(', ') }) : t(key, { count: done }))
  if (invoices.length === 0) {
    return null
  }
  return (
    <div className="bulk-bar" role="group" aria-label={t('invoices.bulk.label')}>
      <span className="bulk-bar__count" role="status">{t('invoices.bulk.selected', { count: invoices.length })}</span>
      <Problems messages={formErrors(remind.error, {}).general} />
      <div className="bulk-bar__actions">
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
    </div>
  )
}
