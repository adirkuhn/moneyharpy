import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { useTaxRates } from '../bookkeeping/queries'
import { contactName } from '../contacts/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors } from '../forms/Form'
import { useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { InvoiceAttachments, InvoiceNotes } from './InvoiceExtras'
import {
  invoiceFileURL, invoiceTitle, isClosed, isSent, type SalesInvoice, useCreditSalesInvoice, useDeleteSalesInvoice, usePauseSalesInvoice, useSalesInvoice,
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
  const [sending, setSending] = useState(false)

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
  const file = (kind: Parameters<typeof invoiceFileURL>[2]) => invoiceFileURL(administrationId, inv.id ?? '', kind)
  const problems = formErrors(remove.error ?? credit.error ?? pause.error, {}).general

  const actions = (
    <>
      {!closed && <Link className="btn btn--secondary btn--small" to={`${list}/${inv.id}/edit`}>{t('invoices.edit')}</Link>}
      {inv.state !== 'uncollectible' && !sending && (
        <button type="button" className="btn btn--primary btn--small" onClick={() => setSending(true)}>{sent ? t('invoices.sendAgain') : t('invoices.send')}</button>
      )}
      <a className="btn btn--secondary btn--small" href={file('download_pdf')} download>{t('invoices.downloadPdf')}</a>
      {sent && <a className="btn btn--secondary btn--small" href={file('download_ubl')} download>{t('invoices.downloadUbl')}</a>}
      <a className="btn btn--secondary btn--small" href={file('download_packing_slip_pdf')} download>{t('invoices.packingSlip')}</a>
      {sent && (
        <button type="button" className="btn btn--secondary btn--small" disabled={credit.isPending} onClick={() => credit.mutate(undefined, { onSuccess: (c) => navigate(`${list}/${c.id}`) })}>
          {t('invoices.credit')}
        </button>
      )}
      {sent && !closed && (
        <button type="button" className="btn btn--secondary btn--small" disabled={pause.isPending} onClick={() => pause.mutate(!inv.paused)}>
          {inv.paused ? t('invoices.resume') : t('invoices.pause')}
        </button>
      )}
      {!sent && <ConfirmDelete label={t('invoices.delete')} confirmLabel={t('invoices.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })} />}
    </>
  )

  return (
    <Page title={invoiceTitle(inv, t)} subtitle={inv.contact ? contactName(inv.contact) : undefined} back={{ to: list, label: t('invoices.backToList') }} actions={actions}>
      <Problems messages={problems} />
      {inv.original_sales_invoice_id && (
        <p className="notice">
          {t('invoices.creditOf')} <Link to={`${list}/${inv.original_sales_invoice_id}`}>{t('invoices.original')}</Link>.
        </p>
      )}
      {inv.paused && sent && !closed && <p className="notice">{t('invoices.paused')}</p>}
      {sending && <SendPanel administrationId={administrationId} invoice={inv} onDone={() => setSending(false)} />}
      <Details administrationId={administrationId} invoice={inv} />
      <Lines administrationId={administrationId} invoice={inv} />
      <InvoiceNotes administrationId={administrationId} invoice={inv} />
      <InvoiceAttachments administrationId={administrationId} invoice={inv} />
    </Page>
  )
}

function Details({ administrationId, invoice: inv }: { administrationId: string; invoice: SalesInvoice }) {
  const { t, i18n } = useTranslation()
  const workflows = useWorkflows(administrationId)
  const date = (d: string | null | undefined) => d && new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const rows: [string, React.ReactNode][] = [
    [t('invoices.status'), <StateBadge key="state" state={inv.state} />],
    [t('invoices.contact'), inv.contact && <Link to={`/${administrationId}/contacts/${inv.contact_id}`}>{contactName(inv.contact)}</Link>],
    [t('invoices.invoiceDate'), date(inv.invoice_date) || t('invoices.dateOnSend')],
    [t('invoices.dueDate'), sentDue(inv) && date(inv.due_date)],
    [t('invoices.reference'), inv.reference],
    [t('invoices.paymentReference'), inv.payment_reference],
    [t('invoices.workflow'), (workflows.data ?? []).find((w) => w.id === inv.workflow_id)?.name],
    [t('invoices.paymentConditions'), inv.payment_conditions],
  ]
  return (
    <dl className="details">
      {rows.filter(([, v]) => v).map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
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
    <section className="invoice-view" aria-labelledby="invoice-lines">
      <h2 id="invoice-lines">{inv.prices_are_incl_tax ? t('invoices.linesInclTax') : t('invoices.linesExclTax')}</h2>
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
    </section>
  )
}
