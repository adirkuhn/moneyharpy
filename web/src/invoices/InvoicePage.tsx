import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { ConfirmPanel } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { ActionMenu, type MenuItem, Unavailable } from '../list/ListActions'
import { ReminderPanel, WriteOffPanel } from './CollectPanels'
import { InvoiceDocument } from './InvoiceDocument'
import { AttachmentDropZone } from './InvoiceExtras'
import { InvoiceSidebar } from './InvoiceSidebar'
import type { Opening } from './InvoiceFormPage'
import {
  invoiceFileURL, invoiceTitle, isClosed, isPayable, isSent, type SalesInvoice, useCreditSalesInvoice, useDeleteSalesInvoice, useDuplicateSalesInvoice,
  usePauseSalesInvoice, useSalesInvoice,
} from './queries'
import { SendPanel } from './SendPanel'
import { StateBadge } from './StateBadge'
import './view.css'

/** One sales invoice with what can be done with it (/sales_invoices/:id). */
export function InvoicePage() {
  const { administrationId = '', invoiceId = '' } = useParams()
  // A fresh page per invoice, so a credit invoice opens with the send panel closed.
  return <InvoiceView key={invoiceId} administrationId={administrationId} invoiceId={invoiceId} />
}

type Panel = 'send' | 'remind' | 'dubious' | 'uncollectible' | null

function InvoiceView({ administrationId, invoiceId }: { administrationId: string; invoiceId: string }) {
  const invoice = useSalesInvoice(administrationId, invoiceId)
  if (invoice.error instanceof ApiError && invoice.error.status < 500) {
    return <NotFoundPage />
  }
  if (!invoice.data) {
    return <Loading />
  }
  return <InvoiceScreen administrationId={administrationId} invoice={invoice.data} />
}

/**
 * The invoice after Moneybird's invoice page: the action bar with Send,
 * Register payment, Download as, Journal entries and Extra and the state at
 * its right, the invoice as its document with its attachments, and the
 * sidebar with what is paid, the contact and the notes and history.
 */
function InvoiceScreen({ administrationId, invoice: inv }: { administrationId: string; invoice: SalesInvoice }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const remove = useDeleteSalesInvoice(administrationId, inv.id ?? '')
  const credit = useCreditSalesInvoice(administrationId, inv.id ?? '')
  const pause = usePauseSalesInvoice(administrationId, inv.id ?? '')
  const duplicate = useDuplicateSalesInvoice(administrationId)
  const [panel, setPanel] = useState<Panel>(null)
  const [confirming, setConfirming] = useState(false)
  const [highlight, setHighlight] = useState<string | undefined>(undefined)
  /** What a block of a sent invoice's document would open, while the warning asks first. */
  const [editing, setEditing] = useState<Opening | undefined>(undefined)

  const list = `/${administrationId}/sales_invoices`
  const draft = inv.state === 'draft'
  const sent = isSent(inv.state)
  const closed = isClosed(inv.state)
  const payable = isPayable(inv.state)
  // The server keeps an invoice as it is once it is paid, or a payment is registered on it.
  const editable = !closed && (inv.payments ?? []).length === 0
  const done = () => setPanel(null)
  const file = (kind: Parameters<typeof invoiceFileURL>[2]) => invoiceFileURL(administrationId, inv.id ?? '', kind)
  const problems = formErrors(remove.error ?? credit.error ?? pause.error ?? duplicate.error, {}).general

  const sendItems: MenuItem[] = [
    ...(inv.state !== 'uncollectible' ? [{ label: sent ? t('invoices.view.sendAgain') : t('invoices.view.sendInvoice'), onSelect: () => setPanel('send') }] : []),
    ...(payable ? [{ label: t('invoices.reminders.action'), onSelect: () => setPanel('remind') }] : []),
  ]
  const downloads: MenuItem[] = [
    { label: t('invoices.view.pdf'), href: file('download_pdf'), download: true },
    ...(sent ? [{ label: t('invoices.view.ubl'), href: file('download_ubl'), download: true }] : []),
    { label: t('invoices.view.packingSlip'), href: file('download_packing_slip_pdf'), download: true },
    ...(inv.state === 'paid' ? [{ label: t('invoices.view.receipt'), unavailable: true as const }] : []),
  ]
  const extra: MenuItem[] = [
    {
      label: t('invoices.view.duplicate'), disabled: duplicate.isPending,
      onSelect: () => duplicate.mutate(inv, { onSuccess: (copy) => navigate(`${list}/${copy.id}/edit`) }),
    },
    { label: t('invoices.view.duplicateQuote'), unavailable: true },
    ...(sent
      ? [{ label: t('invoices.view.duplicateCredit'), disabled: credit.isPending, onSelect: () => credit.mutate(undefined, { onSuccess: (c) => navigate(`${list}/${c.id}`) }) }]
      : []),
    { label: t('invoices.view.duplicateRecurring'), to: `/${administrationId}/recurring_sales_invoices/new?from=${inv.id}` },
    ...(sent ? [{ label: t('invoices.view.showOnline'), unavailable: true as const }] : []),
    ...(sent && !closed ? [{ label: inv.paused ? t('invoices.resume') : t('invoices.pause'), disabled: pause.isPending, onSelect: () => pause.mutate(!inv.paused) }] : []),
    ...(payable && !inv.marked_dubious_on ? [{ label: t('invoices.writeOff.dubious'), onSelect: () => setPanel('dubious') }] : []),
    ...(payable ? [{ label: t('invoices.writeOff.uncollectible'), onSelect: () => setPanel('uncollectible') }] : []),
    // A sent invoice is credited instead, as the server keeps it.
    ...(!sent ? [{ label: t('invoices.view.delete'), danger: true, onSelect: () => setConfirming(true) }] : []),
  ]

  const actions = (
    <div className="invoice-actions">
      <div className="invoice-actions__buttons">
        {sendItems.length > 0 && <ActionMenu primary={draft} label={t('invoices.view.send')} items={sendItems} />}
        {draft && (
          <Link className="btn btn--secondary btn--with-icon" to={`${list}/${inv.id}/edit`}>
            <ListIcon kind="edit" />
            <span>{t('invoices.view.edit')}</span>
          </Link>
        )}
        {payable && <Link className="btn btn--secondary" to={`${list}/${inv.id}/payments/new`}>{t('invoices.view.registerPayment')}</Link>}
        <ActionMenu label={t('invoices.view.downloadAs')} items={downloads} />
        {sent && <Unavailable icon="scale" label={t('invoices.view.journalEntries')} />}
        <ActionMenu label={t('invoices.view.extra')} items={extra} />
      </div>
      <span className="invoice-actions__state"><StateBadge state={inv.state} /></span>
    </div>
  )

  return (
    <Page wide hideTitle title={invoiceTitle(inv, t)} actions={actions}>
      <div className="invoice-view">
        <div className="invoice-view__main">
          <Problems messages={problems} />
          {confirming && (
            <ConfirmPanel
              message={t('invoices.confirmDeleteText')}
              confirmLabel={t('invoices.confirmDelete')}
              disabled={remove.isPending}
              onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
              onCancel={() => setConfirming(false)}
            />
          )}
          {inv.original_sales_invoice_id && (
            <p className="notice">
              {t('invoices.creditOf')} <Link to={`${list}/${inv.original_sales_invoice_id}`}>{t('invoices.original')}</Link>.
            </p>
          )}
          {inv.paused && sent && !closed && <p className="notice">{t('invoices.paused')}</p>}
          {inv.marked_dubious_on && inv.state !== 'paid' && inv.state !== 'uncollectible' && <p className="notice">{t('invoices.writeOff.dubiousNotice')}</p>}
          {panel === 'send' && <SendPanel administrationId={administrationId} invoice={inv} onDone={done} />}
          {panel === 'remind' && <ReminderPanel administrationId={administrationId} invoice={inv} onDone={done} />}
          {(panel === 'dubious' || panel === 'uncollectible') && <WriteOffPanel key={panel} administrationId={administrationId} invoice={inv} kind={panel} onDone={done} />}
          {editing !== undefined && (
            <ConfirmPanel
              danger={false}
              message={t('invoices.view.sentWarning.title')}
              description={t('invoices.view.sentWarning.text')}
              confirmLabel={t('invoices.view.sentWarning.continue')}
              onConfirm={() => navigate(`${list}/${inv.id}/edit`, { state: editing })}
              onCancel={() => setEditing(undefined)}
            />
          )}
          <InvoiceDocument
            administrationId={administrationId}
            invoice={inv}
            editable={editable}
            highlight={highlight}
            onEdit={sent ? setEditing : undefined}
          />
          <AttachmentDropZone administrationId={administrationId} invoice={inv} />
        </div>
        <InvoiceSidebar administrationId={administrationId} invoice={inv} onHighlight={setHighlight} />
      </div>
    </Page>
  )
}
