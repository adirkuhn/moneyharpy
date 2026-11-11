import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { ConfirmPanel, DetailBlock, DetailList, type DetailRow } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type MenuItem, MoreMenu } from '../list/ListActions'
import { ReminderPanel, WriteOffPanel } from './CollectPanels'
import { InvoiceAttachments, InvoiceNotes } from './InvoiceExtras'
import { PaymentsPanel } from './PaymentsPanel'
import {
  invoiceFileURL, invoiceTitle, isClosed, isPayable, isSent, type SalesInvoice, useCreditSalesInvoice, useDeleteSalesInvoice, usePauseSalesInvoice, useSalesInvoice,
} from './queries'
import { SendPanel } from './SendPanel'
import { StateBadge } from './StateBadge'
import { TotalsTable } from './TotalsTable'

/** One sales invoice with what can be done with it (/sales_invoices/:id). */
export function InvoicePage() {
  const { administrationId = '', invoiceId = '' } = useParams()
  // A fresh page per invoice, so a credit invoice opens with the send panel closed.
  return <InvoiceView key={invoiceId} administrationId={administrationId} invoiceId={invoiceId} />
}

function InvoiceView({ administrationId, invoiceId }: { administrationId: string; invoiceId: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const invoice = useSalesInvoice(administrationId, invoiceId)
  const remove = useDeleteSalesInvoice(administrationId, invoiceId)
  const credit = useCreditSalesInvoice(administrationId, invoiceId)
  const pause = usePauseSalesInvoice(administrationId, invoiceId)
  const [panel, setPanel] = useState<'send' | 'remind' | 'dubious' | 'uncollectible' | null>(null)
  const [confirming, setConfirming] = useState(false)

  if (invoice.error instanceof ApiError && invoice.error.status < 500) {
    return <NotFoundPage />
  }
  if (!invoice.data) {
    return <Loading />
  }
  const inv = invoice.data
  const list = `/${administrationId}/sales_invoices`
  const sent = isSent(inv.state)
  const closed = isClosed(inv.state)
  const payable = isPayable(inv.state)
  const done = () => setPanel(null)
  const file = (kind: Parameters<typeof invoiceFileURL>[2]) => invoiceFileURL(administrationId, inv.id ?? '', kind)
  const problems = formErrors(remove.error ?? credit.error ?? pause.error, {}).general

  // What does not need a button of its own goes under ⋯, as on Moneybird.
  const more: MenuItem[] = [
    ...(payable ? [{ label: t('invoices.reminders.action'), onSelect: () => setPanel('remind') }] : []),
    ...(sent ? [{ label: t('invoices.downloadUbl'), href: file('download_ubl'), download: true }] : []),
    { label: t('invoices.packingSlip'), href: file('download_packing_slip_pdf'), download: true },
    ...(sent ? [{ label: t('invoices.credit'), disabled: credit.isPending, onSelect: () => credit.mutate(undefined, { onSuccess: (c) => navigate(`${list}/${c.id}`) }) }] : []),
    ...(sent && !closed ? [{ label: inv.paused ? t('invoices.resume') : t('invoices.pause'), disabled: pause.isPending, onSelect: () => pause.mutate(!inv.paused) }] : []),
    ...(payable && !inv.marked_dubious_on ? [{ label: t('invoices.writeOff.dubious'), onSelect: () => setPanel('dubious') }] : []),
    ...(payable ? [{ label: t('invoices.writeOff.uncollectible'), onSelect: () => setPanel('uncollectible') }] : []),
    ...(inv.state === 'draft' ? [{ label: t('recurring.makeRecurring'), to: `/${administrationId}/recurring_sales_invoices/new?from=${inv.id}` }] : []),
    ...(!sent ? [{ label: t('invoices.delete'), danger: true, onSelect: () => setConfirming(true) }] : []),
  ]
  const actions = (
    <>
      {!closed && <Link className="btn btn--secondary" to={`${list}/${inv.id}/edit`}>{t('invoices.edit')}</Link>}
      <a className="btn btn--secondary" href={file('download_pdf')} download>{t('invoices.downloadPdf')}</a>
      {inv.state !== 'uncollectible' && panel !== 'send' && (
        <button type="button" className="btn btn--primary" onClick={() => setPanel('send')}>{sent ? t('invoices.sendAgain') : t('invoices.send')}</button>
      )}
      <MoreMenu items={more} />
    </>
  )

  return (
    <Page title={invoiceTitle(inv, t)} subtitle={inv.contact ? contactName(inv.contact) : undefined} back={{ to: list, label: t('invoices.backToList') }} actions={actions}>
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
      {panel === 'send' && <SendPanel administrationId={administrationId} invoice={inv} onDone={done} />}
      {panel === 'remind' && <ReminderPanel administrationId={administrationId} invoice={inv} onDone={done} />}
      {(panel === 'dubious' || panel === 'uncollectible') && <WriteOffPanel key={panel} administrationId={administrationId} invoice={inv} kind={panel} onDone={done} />}
      {inv.marked_dubious_on && inv.state !== 'paid' && inv.state !== 'uncollectible' && <p className="notice">{t('invoices.writeOff.dubiousNotice')}</p>}
      <Details administrationId={administrationId} invoice={inv} />
      <Lines administrationId={administrationId} invoice={inv} />
      {sent && <PaymentsPanel administrationId={administrationId} invoice={inv} />}
      <InvoiceNotes administrationId={administrationId} invoice={inv} />
      <InvoiceAttachments administrationId={administrationId} invoice={inv} />
    </Page>
  )
}

function Details({ administrationId, invoice: inv }: { administrationId: string; invoice: SalesInvoice }) {
  const { t, i18n } = useTranslation()
  const workflows = useWorkflows(administrationId)
  const date = (d: string | null | undefined) => d && new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const rows: DetailRow[] = [
    [t('invoices.status'), <StateBadge key="state" state={inv.state} />],
    [t('invoices.contact'), inv.contact && <Link to={`/${administrationId}/contacts/${inv.contact_id}`}>{contactName(inv.contact)}</Link>],
    [t('invoices.invoiceDate'), date(inv.invoice_date) || t('invoices.dateOnSend')],
    [t('invoices.dueDate'), sentDue(inv) && date(inv.due_date)],
    [t('invoices.paidAt'), date(inv.paid_at)],
    [t('invoices.nextReminder'), date(inv.next_reminder)],
    [t('invoices.reminderCount'), inv.reminder_count ? String(inv.reminder_count) : ''],
    [t('invoices.markedDubiousOn'), date(inv.marked_dubious_on)],
    [t('invoices.markedUncollectibleOn'), date(inv.marked_uncollectible_on)],
    [t('invoices.reference'), inv.reference],
    [t('invoices.paymentReference'), inv.payment_reference],
    [t('invoices.workflow'), (workflows.data ?? []).find((w) => w.id === inv.workflow_id)?.name],
    [t('invoices.paymentConditions'), inv.payment_conditions],
  ]
  return (
    <DetailBlock title={t('invoices.detailsTitle')}>
      <DetailList rows={rows} />
    </DetailBlock>
  )
}

/** A draft's due date follows from the date it is sent on, so it is not shown yet. */
const sentDue = (inv: SalesInvoice) => isSent(inv.state) || !!inv.invoice_date

function Lines({ administrationId, invoice: inv }: { administrationId: string; invoice: SalesInvoice }) {
  const { t, i18n } = useTranslation()
  const rates = useTaxRates(administrationId)
  const money = (amount: string | number | null | undefined) => formatAmount(amount, inv.currency ?? 'EUR', i18n.language)
  const rateName = (id: string | number | undefined) => rates.data?.find((r) => r.id === String(id))?.name ?? ''
  const discount = Number(inv.total_discount ?? 0)
  const totals = [
    { label: t('invoices.subtotalExcl'), amount: money(inv.total_price_excl_tax) },
    ...(discount !== 0 ? [{ label: t('invoices.discountAmount', { percentage: Number(inv.discount ?? 0) }), amount: money(-discount) }] : []),
    ...(inv.tax_totals ?? []).map((r) => ({ label: rateName(r.tax_rate_id ?? undefined), amount: money(r.tax_amount) })),
    { label: t('invoices.total'), amount: money(inv.total_price_incl_tax), strong: true },
    ...(isSent(inv.state) ? [{ label: t('invoices.unpaid'), amount: money(inv.total_unpaid) }] : []),
  ]
  return (
    <DetailBlock title={inv.prices_are_incl_tax ? t('invoices.linesInclTax') : t('invoices.linesExclTax')}>
      <div className="detail-sheet">
        <table className="data-table invoice-view__lines" aria-label={t('invoices.lines')}>
        <thead>
          <tr>
            <th scope="col">{t('invoices.description')}</th>
            <th scope="col" className="invoice-view__quantity data-table__extra">{t('invoices.quantity')}</th>
            <th scope="col" className="invoice-view__money data-table__extra">{t('invoices.price')}</th>
            <th scope="col" className="invoice-view__rate data-table__extra">{t('invoices.vatRate')}</th>
            <th scope="col" className="invoice-view__money">{t('invoices.amount')}</th>
          </tr>
        </thead>
        <tbody>
          {(inv.details ?? []).map((d) => (
            <tr key={d.id}>
              <td className="invoice-view__description">{d.description}</td>
              <td className="invoice-view__quantity data-table__extra">{d.amount}</td>
              <td className="invoice-view__money data-table__extra">{money(d.price)}</td>
              <td className="invoice-view__rate data-table__extra">{rateName(d.tax_rate_id)}</td>
              <td className="invoice-view__money">{money(d.total_price_excl_tax_with_discount)}</td>
            </tr>
          ))}
        </tbody>
        </table>
        <TotalsTable rows={totals} />
      </div>
    </DetailBlock>
  )
}
