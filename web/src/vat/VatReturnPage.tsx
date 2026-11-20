import { ActionLink } from '../layout/ActionLink'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ApiError, unwrap } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { ConfirmPanel } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { BoxTable } from './BoxTable'
import { IcpTable } from './IcpTable'
import { periodName } from './period'
import { useDeleteVatReturn, useVatReturn, type VatReturn } from './queries'
import { stateLook } from './VatReturnsPage'
import '../invoices/view.css'
import './vat.css'

/** Days from one yyyy-mm-dd to another. */
const daysBetween = (from: string, to: string) => Math.round((Date.parse(to) - Date.parse(from)) / 86_400_000)

/**
 * Moneybird's VAT return (/declarations/:id): the state block with what is
 * to be paid or received and by when, the card with the facts and the boxes
 * declared with their corrections, and the history beside it; its bookings
 * behind the action bar's scale.
 */
export function VatReturnPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '', returnId = '' } = useParams()
  const navigate = useNavigate()
  const vat = useVatReturn(administrationId, returnId)
  const remove = useDeleteVatReturn(administrationId, returnId)
  const [confirming, setConfirming] = useState(false)
  const [journal, setJournal] = useState(false)

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
      wide
      title={t('vat.view.title', { kind: t(`vat.kinds.${r.kind}`), period: name })}
      actions={
        <div className="accounts-actions">
          <button type="button" className="btn btn--secondary" onClick={() => window.print()}>{t('vat.view.pdf')}</button>
          {r.kind !== 'icp_declaration' && (
            <button
              type="button"
              className={`btn btn--secondary btn--icon${journal ? ' is-active' : ''}`}
              aria-label={t('vat.view.journal')}
              title={t('vat.view.journal')}
              aria-pressed={journal}
              onClick={() => setJournal(!journal)}
            >
              <ListIcon kind="scale" />
            </button>
          )}
          <MoreMenu items={[{ label: t('vat.view.delete'), danger: true, bookkeeping: true, onSelect: () => setConfirming(true) }]} />
        </div>
      }
    >
      <div className="invoice-view">
        <div className="vat-view__main">
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
          {journal && <Bookings administrationId={administrationId} vat={r} />}
          <section className="vat-card" aria-label={t('vat.view.details')}>
            {r.kind !== 'icp_declaration' && <p className="vat-info"><InfoIcon /><span>{t('vat.view.booked')}</span></p>}
            <dl className="vat-facts">
              {r.kind !== 'icp_declaration' && <><dt>{t('vat.view.deadline')}</dt><dd>{r.deadline}</dd></>}
              <dt>{t('vat.sentOn')}</dt><dd>{r.sent_on}</dd>
            </dl>
            {r.kind === 'icp_declaration'
              ? <IcpTable lines={r.icp_lines} label={t('vat.icp.title')} />
              : <BoxTable figures={r} periodName={name} label={t('vat.boxesOf', { period: name })} blankZero />}
          </section>
          <Link className="vat-view__back" to={list}>{t('vat.backToList')}</Link>
        </div>
        <History vat={r} />
      </div>
    </Page>
  )
}

/** Moneybird's dark state block: the amount, how late it is and its state, what to do, and the button to do it. */
function Status({ administrationId, vat }: { administrationId: string; vat: VatReturn }) {
  const { t, i18n } = useTranslation()
  const money = (v: string | number) => new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'EUR' }).format(Math.abs(Number(v)))
  const today = new Date().toISOString().slice(0, 10)
  const overdue = vat.state === 'to_pay' ? daysBetween(vat.deadline, today) : 0
  const amount = money(vat.unpaid)
  const text = {
    to_pay: overdue > 0 ? t('vat.view.overdue', { amount, count: overdue, date: vat.deadline }) : t('vat.view.toPay', { amount, date: vat.deadline }),
    to_receive: t('vat.view.toReceive', { amount }),
    paid: t('vat.view.paid'),
    submitted: t('vat.view.submitted', { date: vat.sent_on }),
  }[vat.state]
  return (
    <section className={`vat-status is-${vat.state}`} aria-label={t('vat.status')}>
      <div className="vat-status__head">
        {vat.kind !== 'icp_declaration' && <span className="vat-status__amount">{money(vat.total)}</span>}
        {overdue > 0 && <span className="vat-status__late">{t('vat.view.daysOverdue', { count: overdue })}</span>}
        <span className={`vat-status__badge is-${stateLook[vat.state] ?? 'open'}`}>{t(`vat.states.${vat.state}`)}</span>
      </div>
      <p className="vat-status__text">{text}</p>
      {(vat.state === 'to_pay' || vat.state === 'to_receive') && (
        <div className="vat-status__actions">
          <ActionLink className="btn btn--secondary" to={`/${administrationId}/financial_mutations`}>{t('vat.view.link')}</ActionLink>
        </div>
      )}
      {vat.payments.length > 0 && (
        <ul className="vat-status__payments">
          {vat.payments.map((p) => (
            <li key={p.id}>
              <Link to={`/${administrationId}/financial_mutations/${p.financial_mutation_id}`}>{t('vat.view.payment', { date: p.payment_date })}</Link>
              <span>{money(p.price)}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

/** What happened to the return, newest first, as Moneybird's history beside it. */
function History({ vat }: { vat: VatReturn }) {
  const { t } = useTranslation()
  const items = [
    ...vat.payments.map((p) => ({ day: p.payment_date, text: t('vat.view.history.paid'), key: p.id })),
    { day: vat.sent_on, text: t('vat.view.history.filed'), key: 'filed' },
    { day: vat.created_at.slice(0, 10), text: t('vat.view.history.created', { kind: t(`vat.kinds.${vat.kind}`) }), key: 'created' },
  ].sort((a, b) => b.day.localeCompare(a.day))
  const days = [...new Set(items.map((i) => i.day))]
  return (
    <aside className="vat-history" aria-label={t('vat.view.history.title')}>
      <h2 className="invoice-history__title">{t('vat.view.history.title')}</h2>
      {days.map((d) => (
        <div key={d} className="history">
          <h3 className="history__day">{d}</h3>
          <ul className="history__list">
            {items.filter((i) => i.day === d).map((i) => (
              <li key={i.key} className="history__row">
                <span className="history__avatar" aria-hidden="true" />
                <p className="history__text">{i.text}</p>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </aside>
  )
}

/** The journal entries the return made: on payable and receivable VAT, VAT paid or received and rounding. */
function Bookings({ administrationId, vat }: { administrationId: string; vat: VatReturn }) {
  const { t, i18n } = useTranslation()
  const accounts = useLedgerAccounts(administrationId)
  const entries = useQuery({
    queryKey: ['vat-return-entries', administrationId, vat.id, vat.updated_at],
    queryFn: () => unwrap(api.GET('/api/v2/{administration_id}/reports/journal_entries', {
      params: { path: { administration_id: administrationId }, query: { document_type: 'VatDocument', document_id: vat.id } },
    })),
  })
  const money = (v: number) => (v > 0 ? new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'EUR' }).format(v) : '')
  const nameOf = (id: string | undefined) => accounts.data?.find((a) => a.id === id)?.name ?? ''
  if (!entries.data) {
    return <Loading />
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
