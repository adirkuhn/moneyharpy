import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { ContactHoverCard } from '../contacts/ContactHoverCard'
import { useContact } from '../contacts/queries'
import { ConfirmPanel } from '../detail/Detail'
import { formErrors, TextArea, TextInput } from '../forms/Form'
import { type HistoryItem, historyOf } from '../invoices/history'
import { NotesAndHistory } from '../invoices/InvoiceHistory'
import { AttachmentZone } from '../invoices/InvoiceExtras'
import { InvoiceEditor, type Opening } from '../invoices/InvoiceFormPage'
import { CategoryChips } from '../invoices/InvoiceSidebar'
import { invoiceTitle, useSalesInvoice } from '../invoices/queries'
import { useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { ActionMenu, type MenuItem } from '../list/ListActions'
import { EstimateBadge } from './EstimatesPage'
import {
  type Estimate, estimateAttachmentURL, estimatePdfURL, estimateTitle, isBillable, isEditable, transitions, useAddEstimateAttachment, useAddEstimateNote,
  useBillEstimate, useChangeEstimateState, useDeleteEstimate, useDeleteEstimateAttachment, useDeleteEstimateNote, useDuplicateEstimate, useEstimate,
  useSendEstimate,
} from './queries'
import '../invoices/view.css'
import './estimates.css'

/** A new quote (/estimates/new, ?contact_id= to start with that contact), or one quote (/estimates/:id). */
export function EstimatePage() {
  const { administrationId = '', estimateId } = useParams()
  if (!estimateId) {
    return <NewEstimate administrationId={administrationId} />
  }
  // A fresh page per quote, so a duplicate opens with its panels closed.
  return <EstimateView key={estimateId} administrationId={administrationId} estimateId={estimateId} />
}

/**
 * Moneybird's "Add quote": the quote drawn as its document and edited in
 * place, with the invoice editor. Once saved it opens on its own page.
 */
function NewEstimate({ administrationId }: { administrationId: string }) {
  const [params] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const preset = useContact(administrationId, params.get('contact_id') ?? undefined)
  if (preset.fetchStatus === 'fetching' && !preset.data) {
    return <Loading />
  }
  return (
    <InvoiceEditor
      mode="new"
      administrationId={administrationId}
      invoice={null}
      quote={{ estimate: null }}
      contact={preset.data}
      opening={location.state as Opening}
      onSaved={(saved, opening) => navigate(`/${administrationId}/estimates/${saved.id}`, { state: opening })}
    />
  )
}

function EstimateView({ administrationId, estimateId }: { administrationId: string; estimateId: string }) {
  const estimate = useEstimate(administrationId, estimateId)
  if (estimate.error instanceof ApiError && estimate.error.status < 500) {
    return <NotFoundPage />
  }
  if (!estimate.data) {
    return <Loading />
  }
  return <EstimateScreen administrationId={administrationId} estimate={estimate.data} />
}

type Panel = 'send' | 'delete' | 'archive' | null

/**
 * One quote after Moneybird's quote page: the action bar with Send quote,
 * Bill quote, Download and Extra, the state and Change state at its right;
 * the quote as its document, edited in place, with its attachments; and the
 * sidebar with its contact, the invoice it was billed to, its workflow, its
 * categories and its notes and history.
 */
function EstimateScreen({ administrationId, estimate: e }: { administrationId: string; estimate: Estimate }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const id = e.id ?? ''
  const state = e.state ?? 'draft'
  const list = `/${administrationId}/estimates`
  const here = `${list}/${id}`
  const send = useSendEstimate(administrationId, id)
  const change = useChangeEstimateState(administrationId, id)
  const bill = useBillEstimate(administrationId, id)
  const remove = useDeleteEstimate(administrationId, id)
  const duplicate = useDuplicateEstimate(administrationId)
  const [panel, setPanel] = useState<Panel>(null)
  const [layoutsOpen, setLayoutsOpen] = useState(false)
  const [highlight, setHighlight] = useState<string | undefined>(undefined)
  const editable = isEditable(state)
  const sent = state !== 'draft'
  const problems = formErrors(send.error ?? change.error ?? bill.error ?? remove.error ?? duplicate.error, {}).general

  const sendItems: MenuItem[] = [
    { label: sent ? t('estimates.view.sendAgain') : t('estimates.view.sendEmail'), onSelect: () => setPanel('send') },
    { label: t('estimates.view.markSent'), disabled: send.isPending, onSelect: () => send.mutate({ delivery_method: 'Manual' }) },
  ]
  // Archiving has its own entry under Extra, as at Moneybird.
  const states: MenuItem[] = transitions[state]
    .filter((s) => s !== 'archived')
    .map((s) => ({ label: t(`estimates.states.${s}`), disabled: change.isPending, onSelect: () => change.mutate(s) }))
  const extra: MenuItem[] = [
    {
      label: t('estimates.view.duplicate'), disabled: duplicate.isPending,
      onSelect: () => duplicate.mutate(e, { onSuccess: (copy) => navigate(`${list}/${copy.id}`) }),
    },
    ...(editable ? [{ label: t('invoices.view.changeLayout'), onSelect: () => setLayoutsOpen(true) }] : []),
    ...(state !== 'archived' && sent ? [{ label: t('estimates.view.archive'), onSelect: () => setPanel('archive') }] : []),
    { label: t('estimates.view.delete'), danger: true, onSelect: () => setPanel('delete') },
  ]

  const actions = (
    <div className="invoice-actions">
      <div className="invoice-actions__buttons">
        <ActionMenu primary={!sent} label={t('estimates.view.send')} items={sendItems} />
        {isBillable(state) && (
          <button
            type="button"
            className="btn btn--secondary"
            disabled={bill.isPending}
            onClick={() => bill.mutate(undefined, { onSuccess: (inv) => navigate(`/${administrationId}/sales_invoices/${inv.id}`) })}
          >
            {t('estimates.view.bill')}
          </button>
        )}
        <a className="btn btn--secondary" href={estimatePdfURL(administrationId, id)} download>{t('estimates.view.download')}</a>
        <ActionMenu label={t('invoices.view.extra')} items={extra} />
      </div>
      <span className="invoice-actions__state estimate-actions__state">
        <EstimateBadge state={state} />
        {states.length > 0 && <ActionMenu label={t('estimates.view.changeState')} items={states} />}
      </span>
    </div>
  )

  return (
    <Page wide hideTitle title={estimateTitle(e, t)} actions={actions}>
      <div className="invoice-view">
        <div className="invoice-view__main">
          <Problems messages={problems} />
          {panel === 'delete' && (
            <ConfirmPanel
              message={t('estimates.confirmDeleteText')}
              confirmLabel={t('estimates.confirmDelete')}
              disabled={remove.isPending}
              onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
              onCancel={() => setPanel(null)}
            />
          )}
          {panel === 'archive' && (
            <ConfirmPanel
              message={t('estimates.confirmArchiveText')}
              confirmLabel={t('estimates.confirmArchive')}
              danger={false}
              disabled={change.isPending}
              onConfirm={() => change.mutate('archived', { onSuccess: () => setPanel(null) })}
              onCancel={() => setPanel(null)}
            />
          )}
          {panel === 'send' && <SendPanel administrationId={administrationId} estimate={e} onDone={() => setPanel(null)} />}
          {e.original_estimate_id && (
            <p className="notice">
              {t('estimates.copyOf')} <Link to={`${list}/${e.original_estimate_id}`}>{t('estimates.original')}</Link>.
            </p>
          )}
          {!editable && <p className="notice">{t('estimates.lockedNotice')}</p>}
          {/* A billed or archived quote shows its document as it is; the server no longer changes it. */}
          <div className={editable ? undefined : 'estimate-locked'} inert={!editable}>
            <InvoiceEditor
              mode="view"
              administrationId={administrationId}
              invoice={null}
              quote={{ estimate: e }}
              opening={location.state as Opening}
              layoutsOpen={layoutsOpen}
              onLayoutsClose={() => setLayoutsOpen(false)}
              highlight={highlight}
              onCancel={() => {}}
              onSaved={(_, opening) => navigate(here, { replace: true, state: opening })}
            />
          </div>
          <EstimateAttachments administrationId={administrationId} estimate={e} />
        </div>
        <EstimateSidebar administrationId={administrationId} estimate={e} onHighlight={setHighlight} />
      </div>
    </Page>
  )
}

function EstimateAttachments({ administrationId, estimate: e }: { administrationId: string; estimate: Estimate }) {
  const add = useAddEstimateAttachment(administrationId, e.id ?? '')
  const remove = useDeleteEstimateAttachment(administrationId, e.id ?? '')
  return <AttachmentZone attachments={e.attachments ?? []} add={add} remove={remove} url={(a) => estimateAttachmentURL(administrationId, e.id ?? '', a)} />
}

/** Moneybird's send form of a quote: the address it goes to and a message of its own, else the workflow's. */
function SendPanel({ administrationId, estimate: e, onDone }: { administrationId: string; estimate: Estimate; onDone: () => void }) {
  const { t } = useTranslation()
  const heading = useId()
  const send = useSendEstimate(administrationId, e.id ?? '')
  const [email, setEmail] = useState(e.contact?.send_estimates_to_email || e.contact?.send_invoices_to_email || '')
  const [message, setMessage] = useState('')
  const labels = { email_address: t('estimates.send.email'), email_message: t('estimates.send.message') }
  const errors = formErrors(send.error, labels)
  const submit = (ev: FormEvent) => {
    ev.preventDefault()
    send.mutate({ delivery_method: 'Email', email_address: email.trim(), ...(message.trim() ? { email_message: message } : {}) }, { onSuccess: onDone })
  }
  return (
    <section className="send-form" aria-labelledby={heading}>
      <form onSubmit={submit} noValidate>
        <div className="send-form__intro">
          <h3 id={heading} className="send-form__title">{t('estimates.send.title')}</h3>
          <p className="send-form__text">{t('estimates.send.text')}</p>
        </div>
        <Problems messages={errors.general} />
        <div className="send-form__fields">
          <TextInput label={labels.email_address} type="email" value={email} errors={errors.field('email_address')} onChange={(ev) => setEmail(ev.target.value)} />
          <TextArea label={labels.email_message} help={t('estimates.send.messageHelp')} maxLength={10000} value={message} errors={errors.field('email_message')} onChange={(ev) => setMessage(ev.target.value)} />
        </div>
        <div className="send-form__actions">
          <button className="btn btn--primary" disabled={send.isPending}>{t('estimates.send.submit')}</button>
          <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
        </div>
      </form>
    </section>
  )
}

type SidebarProps = { administrationId: string; estimate: Estimate; onHighlight: (ledgerAccountId: string | undefined) => void }

/** The column beside a quote, after Moneybird's: its contact, the invoice it was billed to and how much, its workflow, categories, notes and history. */
function EstimateSidebar({ administrationId, estimate: e, onHighlight }: SidebarProps) {
  const { t, i18n } = useTranslation()
  const workflows = useWorkflows(administrationId)
  const billed = [...(e.events ?? [])].reverse().find((ev) => ev.action === 'estimate_billed' && ev.link_entity_id)
  const invoice = useSalesInvoice(administrationId, billed?.link_entity_id ? String(billed.link_entity_id) : undefined)
  const add = useAddEstimateNote(administrationId, e.id ?? '')
  const remove = useDeleteEstimateNote(administrationId, e.id ?? '')
  const flow = (workflows.data ?? []).find((w) => w.id === e.workflow_id)
  const money = (v: string | number | null | undefined) => formatAmount(v, e.currency ?? 'EUR', i18n.language)
  const facts: [string, ReactNode][] = [
    [t('invoices.view.contact'), e.contact && <ContactHoverCard administrationId={administrationId} contact={{ ...e.contact, id: e.contact_id }} />],
    [t('estimates.view.invoice'), invoice.data && <Link to={`/${administrationId}/sales_invoices/${invoice.data.id}`}>{invoiceTitle(invoice.data, t)}</Link>],
    [t('estimates.view.invoiced'), invoice.data && t('estimates.view.invoicedOf', { amount: money(invoice.data.total_price_incl_tax), total: money(e.total_price_incl_tax) })],
    [t('invoices.workflow'), flow && <Link to={`/${administrationId}/workflows/${flow.id}`}>{flow.name}</Link>],
  ]
  return (
    <aside className="invoice-sidebar" aria-label={t('estimates.view.sidebar')}>
      <dl className="invoice-sidebar__facts">
        {facts.filter(([, value]) => value).map(([label, value]) => (
          <div key={label} className="invoice-sidebar__fact">
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <CategoryChips administrationId={administrationId} details={e.details ?? []} onHighlight={onHighlight} />
      <NotesAndHistory administrationId={administrationId} items={estimateHistory(e)} currency={e.currency ?? 'EUR'} add={add} remove={remove} />
    </aside>
  )
}

/** What happened to the quote, newest first; a quote made before the server kept events tells only when it was created. */
function estimateHistory(e: Estimate): HistoryItem[] {
  const told = (e.events ?? []).length > 0
  // Without events of its own, an invoice's history would be made up from its dates; a quote's is told here.
  const items = historyOf({ created_at: told ? (e.created_at ?? undefined) : undefined, paid_at: null, notes: e.notes, events: e.events, payments: [] })
  if (told || !e.created_at) {
    return items
  }
  return [...items, { kind: 'event', id: 'derived-created', action: 'estimate_created', at: e.created_at, timed: true, data: {} }]
}
