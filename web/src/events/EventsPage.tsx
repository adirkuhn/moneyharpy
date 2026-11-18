import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { contactName, useContact } from '../contacts/queries'
import { estimateTitle, useEstimate } from '../estimates/queries'
import { useExternalInvoice } from '../external/queries'
import { useDocumentSummary } from '../incoming/documents'
import { invoiceTitle, useSalesInvoice } from '../invoices/queries'
import { ChevronIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { FilterPanel } from '../list/FilterPanel'
import { useUsers } from '../management/queries'
import { recurringTitle, useRecurringInvoice } from '../recurring/queries'
import { useTimeEntry } from '../timeentries/queries'
import { type Event, useEvents } from './queries'
import { eventText, typeName } from './text'
import '../reports/reports.css'
import './events.css'

const periods = ['this_month', 'prev_month', 'this_quarter', 'prev_quarter', 'this_year', 'prev_year'] as const

/** Where a record's page is, for the types that have one. */
const pages: Record<string, string> = {
  SalesInvoice: 'sales_invoices', ExternalSalesInvoice: 'external_sales_invoices', Estimate: 'estimates', Contact: 'contacts',
  Document: 'documents', FinancialMutation: 'financial_mutations', Project: 'projects', RecurringSalesInvoice: 'recurring_sales_invoices', Product: 'products',
  TimeEntry: 'time_entries',
}

/**
 * Moneybird's list of events: everything that happened in the administration
 * in a period, newest first, with what it happened to, who did it and when.
 */
export function EventsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search] = useSearchParams()
  const asked = search.get('period') ?? ''
  const period = (periods as readonly string[]).includes(asked) ? asked : 'this_month'
  // The cursors of the pages seen so far, to go back to the newer ones.
  const [cursors, setCursors] = useState<string[]>([])
  const cursor = cursors.at(-1)
  const events = useEvents(administrationId, period, cursor)
  const users = useUsers(administrationId)
  const names = useMemo(() => new Map((users.data ?? []).map((u) => [String(u.id), u.name ?? ''])), [users.data])
  const label = t('events.label', { period: t(`journal.periods.${period}`).toLowerCase() })
  return (
    <Page
      wide
      title={t('sidenav.items.events')}
      lead={
        <FilterPanel
          label={label}
          title={t('events.filters')}
          quick={periods.map((p) => ({ label: t(`journal.periods.${p}`), to: `/${administrationId}/events?period=${p}`, current: p === period }))}
        />
      }
    >
      {!events.data ? (
        events.error ? <p className="empty">{t('events.loadError')}</p> : <Loading />
      ) : events.data.events.length === 0 && cursors.length === 0 ? (
        <p className="empty">{t('events.empty')}</p>
      ) : (
        <section className="report-card events-card">
          <table className="events-table" aria-label={t('sidenav.items.events')}>
            <thead>
              <tr>
                <th scope="col">{t('events.event')}</th>
                <th scope="col">{t('events.type')}</th>
                <th scope="col">{t('events.user')}</th>
                <th scope="col">{t('events.date')}</th>
                <th scope="col">{t('events.time')}</th>
                <th scope="col"><span className="sr-only">{t('events.initials')}</span></th>
              </tr>
            </thead>
            <tbody>
              {events.data.events.map((e) => <Row key={String(e.id)} administrationId={administrationId} event={e} user={names.get(String(e.user_id)) ?? ''} />)}
            </tbody>
          </table>
          {(cursors.length > 0 || events.data.next) && (
            <nav className="events-pager" aria-label={t('common.pagination')}>
              <button type="button" className="btn btn--secondary btn--icon" disabled={cursors.length === 0} aria-label={t('events.newer')} title={t('events.newer')} onClick={() => setCursors(cursors.slice(0, -1))}>
                <ChevronIcon className="events-pager__back" />
              </button>
              <button
                type="button"
                className="btn btn--secondary btn--icon"
                disabled={!events.data.next}
                aria-label={t('events.older')}
                title={t('events.older')}
                onClick={() => events.data?.next && setCursors([...cursors, events.data.next])}
              >
                <ChevronIcon />
              </button>
            </nav>
          )}
        </section>
      )}
    </Page>
  )
}

/** Initials of a name, as on the avatars: "Jan Voorbeeld" → JV. */
export function initials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return '?'
  }
  return parts.length === 1 ? (parts[0] ?? '').slice(0, 2).toUpperCase() : `${parts[0]?.[0] ?? ''}${parts.at(-1)?.[0] ?? ''}`.toUpperCase()
}

function Row({ administrationId, event: e, user }: { administrationId: string; event: Event; user: string }) {
  const { t, i18n } = useTranslation()
  const at = new Date(e.created_at ?? '')
  const day = Number.isNaN(at.getTime()) ? '' : `${at.getFullYear()}-${String(at.getMonth() + 1).padStart(2, '0')}-${String(at.getDate()).padStart(2, '0')}`
  const time = Number.isNaN(at.getTime()) ? '' : at.toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit', hour12: false })
  // A payment is shown on what it pays, as Moneybird.
  const type = e.link_entity_type && e.link_entity_id ? e.link_entity_type : (e.entity_type ?? '')
  const id = e.link_entity_type && e.link_entity_id ? String(e.link_entity_id) : String(e.entity_id ?? '')
  return (
    <tr>
      <td>{eventText(e.entity_type ?? '', e.action ?? '', t)}</td>
      <td><Subject administrationId={administrationId} type={type} id={id} deleted={(e.action ?? '').endsWith('_destroyed')} /></td>
      <td>{user}</td>
      <td className="events-table__nowrap">{day}</td>
      <td className="events-table__nowrap">{time}</td>
      <td><span className={`events-avatar is-${(Number(String(e.user_id ?? '0').slice(-2)) % 4) + 1}`} aria-hidden="true">{initials(user)}</span></td>
    </tr>
  )
}

/** The record an event happened to, by name and with a link to it where it has a page. */
function Subject({ administrationId, type, id, deleted }: { administrationId: string; type: string; id: string; deleted: boolean }) {
  const { t } = useTranslation()
  const live = !deleted && !!id
  const invoice = useSalesInvoice(administrationId, live && type === 'SalesInvoice' ? id : undefined)
  const external = useExternalInvoice(administrationId, live && type === 'ExternalSalesInvoice' ? id : undefined)
  const estimate = useEstimate(administrationId, live && type === 'Estimate' ? id : undefined)
  const contact = useContact(administrationId, live && type === 'Contact' ? id : undefined)
  const document = useDocumentSummary(administrationId, live && type === 'Document' ? id : '')
  const recurring = useRecurringInvoice(administrationId, live && type === 'RecurringSalesInvoice' ? id : undefined)
  const customer = useContact(administrationId, recurring.data?.contact_id ? String(recurring.data.contact_id) : undefined)
  const entry = useTimeEntry(administrationId, live && type === 'TimeEntry' ? id : undefined)
  const name = invoice.data ? invoiceTitle(invoice.data, t)
    : external.data ? (external.data.reference ?? '')
    : estimate.data ? estimateTitle(estimate.data, t)
    : contact.data ? contactName(contact.data)
    : document.data ? document.data.reference
    : recurring.data ? recurringTitle(recurring.data, customer.data ? contactName(customer.data) : '', t)
    : entry.data ? (entry.data.description ?? '')
    : ''
  const text = name ? `${typeName(type, t)} ${name}` : typeName(type, t)
  const page = pages[type]
  return live && page ? <Link to={`/${administrationId}/${page}/${id}`}>{text}</Link> : <>{text}</>
}
