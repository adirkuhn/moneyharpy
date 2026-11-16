import type { TFunction } from 'i18next'
import { type FormEvent, type ReactNode, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { ContactHoverCard } from '../contacts/ContactHoverCard'
import { useContact } from '../contacts/queries'
import { ConfirmPanel } from '../detail/Detail'
import { formErrors, FormRow, TextArea } from '../forms/Form'
import { type Signature, SignaturePad } from '../forms/SignaturePad'
import { type HistoryItem, historyOf } from '../invoices/history'
import { NotesAndHistory } from '../invoices/InvoiceHistory'
import { AttachmentZone } from '../invoices/InvoiceExtras'
import { InvoiceEditor, type Opening } from '../invoices/InvoiceFormPage'
import { CategoryChips } from '../invoices/InvoiceSidebar'
import { invoiceTitle, useSalesInvoice } from '../invoices/queries'
import { useWorkflows } from '../invoicing/queries'
import { CaretIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { ActionMenu, type MenuItem } from '../list/ListActions'
import { EstimateBadge } from './EstimatesPage'
import {
  type Estimate, estimateAttachmentURL, estimatePdfURL, signedEstimatePdfURL, estimateTitle, isBillable, isEditable, transitions, useAddEstimateAttachment, useAddEstimateNote,
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
    // Moneybird's "View quote online": the page the contact opens, while its code is valid.
    ...(e.online_signature?.online_url ? [{ label: t('estimates.view.viewOnline'), href: e.online_signature.online_url }] : []),
    ...(state !== 'archived' && sent ? [{ label: t('estimates.view.archive'), onSelect: () => setPanel('archive') }] : []),
    { label: t('estimates.view.delete'), danger: true, onSelect: () => setPanel('delete') },
  ]

  const actions = (
    <div className="invoice-actions">
      <div className="invoice-actions__buttons">
        {/* As on Moneybird, Send quote opens its form above the quote rather than a menu. */}
        <button
          type="button"
          className={`btn ${sent ? 'btn--secondary' : 'btn--primary'} btn--with-icon${panel === 'send' ? ' is-active' : ''}`}
          aria-expanded={panel === 'send'}
          onClick={() => setPanel(panel === 'send' ? null : 'send')}
        >
          <span>{t('estimates.view.send')}</span>
          <CaretIcon />
        </button>
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

const methods = ['Email', 'Manual'] as const
type Method = (typeof methods)[number]

/**
 * Moneybird's send form of a quote, above the quote: by email, to one or
 * more addresses with a message starting as the workflow's, or manually,
 * which only marks it sent.
 */
function SendPanel({ administrationId, estimate: e, onDone }: { administrationId: string; estimate: Estimate; onDone: () => void }) {
  const { t, i18n } = useTranslation()
  const heading = useId()
  const helpId = useId()
  const send = useSendEstimate(administrationId, e.id ?? '')
  const workflows = useWorkflows(administrationId)
  const [method, setMethod] = useState<Method>('Email')
  const [email, setEmail] = useState(e.contact?.send_estimates_to_email || e.contact?.send_invoices_to_email || '')
  const [several, setSeveral] = useState(false)
  const [message, setMessage] = useState<string | null>(null)
  const [signOnline, setSignOnline] = useState(e.sign_online ?? true)
  const [selfSign, setSelfSign] = useState(false)
  const [signature, setSignature] = useState<Signature | null>(null)
  const [unsigned, setUnsigned] = useState(false)
  const address = useRef<HTMLInputElement>(null)
  // Sending again replaces the access code of the earlier email, as Moneybird warns.
  const resending = !!e.public_view_code && !e.online_signature?.signed
  const flows = workflows.data ?? []
  const flow = flows.find((w) => w.id === e.workflow_id) ?? flows.find((w) => w.type === 'EstimateWorkflow' && w.default)
  const dt = i18n.getFixedT(e.language || flow?.language || i18n.language)
  const text = message ?? (flow?.invoice_email_body?.trim() || dt('estimates.send.defaultMessage'))
  const labels = { delivery_method: t('estimates.send.method'), email_address: t('estimates.send.email'), email_message: t('estimates.send.message') }
  const errors = formErrors(send.error, labels)

  const addAddress = () => {
    const kept = email.replace(/[\s,]+$/, '')
    setEmail(kept ? `${kept}, ` : '')
    setSeveral(true)
    address.current?.focus()
  }
  const submit = (ev: FormEvent) => {
    ev.preventDefault()
    const signing = method === 'Email' && signOnline && selfSign
    if (signing && !signature) {
      setUnsigned(true)
      return
    }
    const body = method === 'Email'
      ? {
        delivery_method: method, email_address: email.split(',').map((m) => m.trim()).filter(Boolean).join(', '), email_message: text, sign_online: signOnline,
        ...(signing && signature ? { sender: true, signature_output: signature.image } : {}),
      }
      : { delivery_method: method }
    send.mutate(body, { onSuccess: onDone })
  }
  const option = (m: Method) => (
    <label className="send-option">
      <input type="radio" name={`${heading}-method`} value={m} checked={method === m} onChange={() => setMethod(m)} />
      <span className="send-option__text">
        <strong>{t(`estimates.send.methods.${m}`)}</strong> <small>{t(`estimates.send.methodsText.${m}`)}</small>
      </span>
    </label>
  )
  return (
    <section className="send-form send-form--quote" aria-labelledby={heading}>
      <form onSubmit={submit} noValidate>
        <h3 id={heading} className="send-form__title send-form__title--strong">{t('estimates.send.title', { name: estimateTitle(e, t) })}</h3>
        <Problems messages={errors.general} />
        <fieldset className="send-options" aria-label={labels.delivery_method}>
          {option('Email')}
          {method === 'Email' && (
            <div className="send-options__indent">
              <FormRow label={labels.email_address} errors={errors.field('email_address')}>
                {({ labelId: _, ...control }) => (
                  <div className="send-form__emails send-form__emails--wide">
                    <div className="send-form__email">
                      <input
                        ref={address}
                        type="text"
                        autoCapitalize="none"
                        maxLength={1000}
                        value={email}
                        onChange={(ev) => setEmail(ev.target.value)}
                        onBlur={() => setEmail(email.replace(/[\s,]+$/, ''))}
                        {...control}
                        aria-describedby={[control['aria-describedby'], several ? helpId : undefined].filter(Boolean).join(' ') || undefined}
                      />
                      <button type="button" className="send-form__icon-button" aria-label={t('estimates.send.addEmail')} title={t('estimates.send.addEmail')} onClick={addAddress}>
                        <span aria-hidden="true">+</span>
                      </button>
                    </div>
                    {several && <p id={helpId} className="form-input__help">{t('estimates.send.severalHelp')}</p>}
                  </div>
                )}
              </FormRow>
              {resending && <p className="warning" role="note">{t('estimates.send.resendWarning')}</p>}
              <TextArea
                label={labels.email_message}
                rows={10}
                maxLength={10000}
                value={text}
                errors={errors.field('email_message')}
                onChange={(ev) => setMessage(ev.target.value)}
              />
              <p className="send-options__note">{t('estimates.send.onlineText')}</p>
              <label className="check">
                <input type="checkbox" checked={signOnline} onChange={(ev) => setSignOnline(ev.target.checked)} />
                <span>{t('estimates.send.signOnline')}</span>
              </label>
              {signOnline && (
                <>
                  <label className="check">
                    <input type="checkbox" checked={selfSign} onChange={(ev) => setSelfSign(ev.target.checked)} />
                    <span>{t('estimates.send.selfSign')}</span>
                  </label>
                  {selfSign && (
                    <div className="send-options__signature">
                      <SignaturePad
                        label={t('signature.label')}
                        invalid={unsigned && !signature}
                        onChange={(s) => {
                          setSignature(s)
                          setUnsigned(false)
                        }}
                      />
                      {(unsigned && !signature) || errors.field('signature_output') ? (
                        <p className="form-input__message form-input__message--error">{errors.field('signature_output')?.join(' ') || t('signature.required')}</p>
                      ) : null}
                    </div>
                  )}
                </>
              )}
            </div>
          )}
          <hr />
          {option('Manual')}
        </fieldset>
        <div className="send-form__actions send-form__actions--start">
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
    // What was kept of the online signature, as Moneybird lists it beside the quote.
    ...signatureFacts(e, t, i18n.language),
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

/** The contact's online signature: who signed, with which address, from where and when, and the signed PDF. */
function signatureFacts(e: Estimate, t: TFunction, language: string): [string, ReactNode][] {
  const g = e.online_signature
  if (!g?.signed) {
    return []
  }
  const at = g.signed_at ? new Date(g.signed_at).toLocaleString(language, { dateStyle: 'medium', timeStyle: 'short' }) : ''
  return [
    [t('estimates.signature.signedBy'), g.signer_name],
    [t('estimates.signature.email'), g.signer_email],
    [t('estimates.signature.ip'), g.signer_ip],
    [t('estimates.signature.signedAt'), at],
    [t('estimates.signature.document'), <a href={signedEstimatePdfURL(e.administration_id ?? '', e.id ?? '')} download>{t('estimates.signature.download')}</a>],
  ]
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
