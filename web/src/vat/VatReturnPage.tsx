import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ApiError, unwrap } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { ConfirmPanel } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { BoxTable, useEuros } from './BoxTable'
import { IcpTable } from './IcpTable'
import { periodName } from './period'
import { useDeleteVatReturn, useVatReturn, type VatReturn } from './queries'
import { stateLook } from './VatReturnsPage'
import './vat.css'

/** Days from one yyyy-mm-dd to another. */
const daysBetween = (from: string, to: string) => Math.round((Date.parse(to) - Date.parse(from)) / 86_400_000)

/**
 * Moneybird's VAT return (/declarations/:id): what is to be paid or
 * received and by when, the boxes declared with their corrections, the
 * bookings it made and the payments.
 */
export function VatReturnPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '', returnId = '' } = useParams()
  const navigate = useNavigate()
  const vat = useVatReturn(administrationId, returnId)
  const remove = useDeleteVatReturn(administrationId, returnId)
  const [confirming, setConfirming] = useState(false)

  if (vat.error instanceof ApiError && vat.error.status < 500) {
    return <NotFoundPage />
  }
  if (!vat.data) {
    return <Loading />
  }
  const r = vat.data
  const list = `/${administrationId}/declarations`
  const name = periodName(r.period_from, r.period_to, t, i18n.language)
  return (
    <Page
      title={t('vat.view.title', { kind: t(`vat.kinds.${r.kind}`), period: name })}
      back={{ to: list, label: t('vat.backToList') }}
      actions={
        <div className="accounts-actions">
          <button type="button" className="btn btn--secondary" onClick={() => window.print()}>{t('vat.view.pdf')}</button>
          <MoreMenu items={[{ label: t('vat.view.delete'), danger: true, onSelect: () => setConfirming(true) }]} />
        </div>
      }
    >
      <Problems messages={formErrors(remove.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('vat.view.confirmDeleteText')}
          confirmLabel={t('vat.view.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      <Status administrationId={administrationId} vat={r} />
      {r.kind === 'icp_declaration'
        ? <IcpTable lines={r.icp_lines} label={t('vat.icp.title')} />
        : <BoxTable figures={r} periodName={name} label={t('vat.boxesOf', { period: name })} />}
      {r.kind !== 'icp_declaration' && <Bookings administrationId={administrationId} vat={r} />}
    </Page>
  )
}

/** The amount to pay or receive, its deadline and payments, as Moneybird's state block. */
function Status({ administrationId, vat }: { administrationId: string; vat: VatReturn }) {
  const { t } = useTranslation()
  const euros = useEuros()
  const today = new Date().toISOString().slice(0, 10)
  const overdue = vat.state === 'to_pay' ? daysBetween(vat.deadline, today) : 0
  const amount = euros(Math.abs(Number(vat.unpaid)))
  const text = {
    to_pay: overdue > 0 ? t('vat.view.overdue', { amount, count: overdue, date: vat.deadline }) : t('vat.view.toPay', { amount, date: vat.deadline }),
    to_receive: t('vat.view.toReceive', { amount }),
    paid: t('vat.view.paid'),
    submitted: t('vat.view.submitted', { date: vat.sent_on }),
  }[vat.state]
  return (
    <section className="vat-status" aria-label={t('vat.status')}>
      <div className="vat-status__head">
        {vat.kind !== 'icp_declaration' && <span className="vat-status__amount">{euros(Math.abs(Number(vat.total)))}</span>}
        {overdue > 0 && <span className="vat-status__late">{t('vat.view.daysOverdue', { count: overdue })}</span>}
        <span className={`badge badge--${stateLook[vat.state] ?? 'open'} document-table__state`}>{t(`vat.states.${vat.state}`)}</span>
      </div>
      <p className="vat-status__text">{text}</p>
      {(vat.state === 'to_pay' || vat.state === 'to_receive') && (
        <Link className="btn btn--secondary" to={`/${administrationId}/financial_mutations`}>{t('vat.view.link')}</Link>
      )}
      <dl className="vat-status__facts">
        {vat.kind !== 'icp_declaration' && <><dt>{t('vat.view.deadline')}</dt><dd>{vat.deadline}</dd></>}
        <dt>{t('vat.sentOn')}</dt><dd>{vat.sent_on}</dd>
        {vat.payments.map((p) => (
          <div key={p.id} className="vat-status__payment">
            <dt>{t('vat.view.payment', { date: p.payment_date })}</dt>
            <dd><Link to={`/${administrationId}/financial_mutations/${p.financial_mutation_id}`}>{euros(Math.abs(Number(p.price)))}</Link></dd>
          </div>
        ))}
      </dl>
      {vat.kind !== 'icp_declaration' && <p className="vat-status__note">{t('vat.view.booked')}</p>}
    </section>
  )
}

/** The journal entries the return made: on payable and receivable VAT, VAT paid or received and rounding. */
function Bookings({ administrationId, vat }: { administrationId: string; vat: VatReturn }) {
  const { t, i18n } = useTranslation()
  const accounts = useLedgerAccounts(administrationId)
  const entries = useQuery({
    queryKey: ['vat-return-entries', administrationId, vat.id, vat.updated_at],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/journal_entries', {
      params: {
        path: { administration_id: administrationId },
        query: { document_type: 'VatDocument', document_id: vat.id },
      },
    })),
  })
  const money = (v: number) => (v > 0 ? new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'EUR' }).format(v) : '')
  const nameOf = (id: string | undefined) => accounts.data?.find((a) => a.id === id)?.name ?? ''
  if (!entries.data || entries.data.length === 0) {
    return null
  }
  return (
    <table className="data-table vat-bookings" aria-label={t('vat.view.journal')}>
      <caption>{t('vat.view.journal')}</caption>
      <thead>
        <tr>
          <th scope="col">{t('vat.view.category')}</th>
          <th scope="col" className="vat-boxes__amount">{t('vat.view.debit')}</th>
          <th scope="col" className="vat-boxes__amount">{t('vat.view.credit')}</th>
        </tr>
      </thead>
      <tbody>
        {entries.data.map((e) => (
          <tr key={e.id}>
            <td>{nameOf(e.ledger_account_id)}</td>
            <td className="vat-boxes__amount">{money(Number(e.amount))}</td>
            <td className="vat-boxes__amount">{money(-Number(e.amount))}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
