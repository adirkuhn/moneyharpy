import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { contactName } from '../contacts/queries'
import { ConfirmPanel, DetailBlock, DetailList, type DetailRow } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { StateBadge } from '../invoices/StateBadge'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { type IncomingDocument, type IncomingKind, incomingAttachmentURL, incomingTitle, useDeleteIncomingDocument, useIncomingDocument } from './queries'

/** One purchase invoice or receipt (/documents/purchase_invoices/:id, /documents/receipts/:id), with its lines, payments and files. */
export function IncomingDocumentPage({ kind }: { kind: IncomingKind }) {
  const { administrationId = '', documentId = '' } = useParams()
  const doc = useIncomingDocument(administrationId, kind, documentId)
  if (doc.error instanceof ApiError && doc.error.status < 500) {
    return <NotFoundPage />
  }
  if (!doc.data) {
    return <Loading />
  }
  return <DocumentView key={doc.data.id} administrationId={administrationId} kind={kind} doc={doc.data} />
}

function DocumentView({ administrationId, kind, doc }: { administrationId: string; kind: IncomingKind; doc: IncomingDocument }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const remove = useDeleteIncomingDocument(administrationId, kind, doc.id ?? '')
  const [confirming, setConfirming] = useState(false)
  const list = `/${administrationId}/documents/${kind}`
  const currency = doc.currency ?? 'EUR'
  const money = (amount: string | null | undefined) => formatAmount(amount ?? '0', currency, i18n.language)
  const date = (d: string | null | undefined) => d && new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const rows: DetailRow[] = [
    [t('invoices.status'), <StateBadge key="state" state={doc.state} />],
    [t('invoices.contact'), doc.contact && <Link to={`/${administrationId}/contacts/${doc.contact_id}`}>{contactName(doc.contact)}</Link>],
    [t('purchases.date'), date(doc.date)],
    [t('invoices.dueDate'), date(doc.due_date)],
    [t('invoices.paidAt'), date(doc.paid_at)],
    [t('invoices.reference'), doc.reference],
    [t('purchases.total'), money(doc.total_price_incl_tax)],
    [t('purchases.unpaid'), money(doc.total_unpaid)],
  ]

  return (
    <Page
      title={incomingTitle(doc, kind, t)}
      subtitle={doc.contact ? contactName(doc.contact) : undefined}
      back={{ to: list, label: t(`purchases.backTo.${kind}`) }}
      actions={<MoreMenu items={[{ label: t('purchases.delete'), danger: true, onSelect: () => setConfirming(true) }]} />}
    >
      <Problems messages={formErrors(remove.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('purchases.confirmDelete')}
          confirmLabel={t('purchases.delete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      {doc.state === 'new' && <p className="notice">{t('purchases.newNotice')}</p>}
      <DetailBlock title={t('purchases.details')}>
        <DetailList rows={rows} />
      </DetailBlock>
      <DetailBlock title={t('purchases.lines')}>
        {(doc.details ?? []).length === 0 ? (
          <p className="empty">{t('purchases.noLines')}</p>
        ) : (
          <table className="data-table invoice-lines" aria-label={t('purchases.lines')}>
            <thead>
              <tr>
                <th>{t('purchases.description')}</th>
                <th className="transactions__amount">{t('purchases.price')}</th>
                <th className="transactions__amount">{t('purchases.excl')}</th>
              </tr>
            </thead>
            <tbody>
              {(doc.details ?? []).map((d) => (
                <tr key={d.id}>
                  <td>{d.description}</td>
                  <td className="transactions__amount">{money(String(d.price ?? 0))}</td>
                  <td className="transactions__amount">{money(String(d.total_price_excl_tax_with_discount ?? 0))}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </DetailBlock>
      <DetailBlock title={t('purchases.payments')}>
        {(doc.payments ?? []).length === 0 ? (
          <p className="empty">{t('purchases.noPayments')}</p>
        ) : (
          <ul>
            {(doc.payments ?? []).map((p) => (
              <li key={p.id}>{date(p.payment_date)} — {money(String(p.price ?? '0'))}</li>
            ))}
          </ul>
        )}
      </DetailBlock>
      {(doc.attachments ?? []).length > 0 && (
        <DetailBlock title={t('list.attachments')}>
          <ul>
            {(doc.attachments ?? []).map((a) => (
              <li key={a.id}>
                <a href={incomingAttachmentURL(administrationId, kind, doc.id ?? '', a.id ?? '')} target="_blank" rel="noreferrer">{a.filename}</a>
              </li>
            ))}
          </ul>
        </DetailBlock>
      )}
    </Page>
  )
}
