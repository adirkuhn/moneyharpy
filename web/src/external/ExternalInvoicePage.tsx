import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { type TaxRate, useTaxRates } from '../bookkeeping/queries'
import { ConfirmPanel } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { WriteOffPanel } from '../invoices/CollectPanels'
import { InvoiceHistory } from '../invoices/InvoiceHistory'
import { CategoryChips } from '../invoices/InvoiceSidebar'
import { JournalEntriesPanel } from '../invoices/JournalEntriesPanel'
import { isPayable } from '../invoices/queries'
import { StateBadge } from '../invoices/StateBadge'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { PageIcon } from '../layout/pageIcons'
import { MoreMenu } from '../list/ListActions'
import { AttachmentPreviews } from './AttachmentPreviews'
import { ExternalDocument } from './ExternalDocument'
import { ExternalFormPage } from './ExternalFormPage'
import { type ExternalInvoice, externalTitle, isEditable, useDeleteExternalInvoice, useExternalInvoice } from './queries'
import '../invoices/view.css'
import './external.css'

/**
 * One external invoice (/external_sales_invoices/:id). A booked one reads as
 * Moneybird's document; a new one, such as an uploaded file, opens on its
 * edit page to be filled in.
 */
export function ExternalInvoicePage() {
  const { administrationId = '', invoiceId = '' } = useParams()
  const invoice = useExternalInvoice(administrationId, invoiceId)
  const rates = useTaxRates(administrationId)
  if (invoice.error instanceof ApiError && invoice.error.status < 500) {
    return <NotFoundPage />
  }
  if (!invoice.data || !rates.data) {
    return <Loading />
  }
  if (invoice.data.state === 'new') {
    return <ExternalFormPage key={invoice.data.id} administrationId={administrationId} invoice={invoice.data} />
  }
  return <ExternalInvoiceScreen key={invoice.data.id} administrationId={administrationId} invoice={invoice.data} rates={rates.data} />
}

type Panel = 'journal' | 'dubious' | 'uncollectible' | null

/**
 * After Moneybird's external invoice page: the action bar with Register
 * payment, Edit, Journal entries and Delete and the state at its right, the
 * invoice's summary and lines with its files under it, and the sidebar with
 * its categories, notes and history.
 */
function ExternalInvoiceScreen({ administrationId, invoice: inv, rates }: { administrationId: string; invoice: ExternalInvoice; rates: TaxRate[] }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const remove = useDeleteExternalInvoice(administrationId, inv.id ?? '')
  const [panel, setPanel] = useState<Panel>(null)
  const [confirming, setConfirming] = useState(false)
  const [highlight, setHighlight] = useState<string | undefined>(undefined)
  const list = `/${administrationId}/external_sales_invoices`
  const here = `${list}/${inv.id}`
  const payable = isPayable(inv.state)
  const writeOffs = [
    ...(payable && !inv.marked_dubious_on ? [{ label: t('invoices.writeOff.dubious'), onSelect: () => setPanel('dubious') }] : []),
    ...(payable ? [{ label: t('invoices.writeOff.uncollectible'), onSelect: () => setPanel('uncollectible') }] : []),
  ]

  const actions = (
    <div className="invoice-actions invoice-actions--external">
      <div className="invoice-actions__buttons">
        {payable && <Link className="btn btn--secondary" to={`${here}/payments/new`}>{t('external.view.registerPayment')}</Link>}
        {isEditable(inv) && <Link className="btn btn--secondary" to={`${here}/edit`}>{t('external.view.edit')}</Link>}
        <button
          type="button"
          className="btn btn--secondary btn--icon invoice-actions__toggle"
          aria-label={t('invoices.view.journalEntries')}
          title={t('invoices.view.journalEntries')}
          aria-pressed={panel === 'journal'}
          onClick={() => setPanel(panel === 'journal' ? null : 'journal')}
        >
          <ListIcon kind="scale" />
          <svg className="invoice-actions__caret" viewBox="0 0 24 24" width="0.875em" height="0.875em" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <path d={panel === 'journal' ? 'm6 15 6-6 6 6' : 'm6 9 6 6 6-6'} />
          </svg>
        </button>
        <button
          type="button"
          className="btn btn--secondary btn--icon"
          aria-label={t('external.view.delete')}
          title={t('external.view.delete')}
          onClick={() => setConfirming(true)}
        >
          <PageIcon kind="trash" />
        </button>
        {writeOffs.length > 0 && <MoreMenu items={writeOffs} />}
      </div>
      <span className="invoice-actions__state"><StateBadge state={inv.state} /></span>
    </div>
  )

  return (
    <Page wide hideTitle title={t('external.view.title', { name: externalTitle(inv, t) })} actions={actions}>
      <div className="invoice-view external-view">
        <div className="invoice-view__main">
          <Problems messages={formErrors(remove.error, {}).general} />
          {confirming && (
            <ConfirmPanel
              message={t('external.confirmDeleteText')}
              confirmLabel={t('invoices.confirmDelete')}
              disabled={remove.isPending}
              onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
              onCancel={() => setConfirming(false)}
            />
          )}
          {inv.marked_dubious_on && inv.state !== 'paid' && inv.state !== 'uncollectible' && <p className="notice">{t('invoices.writeOff.dubiousNotice')}</p>}
          {(panel === 'dubious' || panel === 'uncollectible') && (
            <WriteOffPanel key={panel} administrationId={administrationId} invoice={inv} kind={panel} invoiceKind="external" onDone={() => setPanel(null)} />
          )}
          {panel === 'journal' && <JournalEntriesPanel administrationId={administrationId} invoice={inv} kind="external" />}
          <ExternalDocument administrationId={administrationId} invoice={inv} rates={rates} highlight={highlight} />
          <AttachmentPreviews administrationId={administrationId} invoice={inv} />
        </div>
        <aside className="invoice-sidebar" aria-label={t('external.view.sidebar')}>
          <CategoryChips administrationId={administrationId} details={inv.details ?? []} onHighlight={setHighlight} />
          <InvoiceHistory administrationId={administrationId} invoice={inv} kind="external" />
        </aside>
      </div>
    </Page>
  )
}
