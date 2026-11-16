import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { groupIBAN } from '../invoices/paper'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { FilterPanel } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import {
  formatAmount, paymentBatchURL, type PurchaseTransaction, useCreatePaymentBatch, useDeletePurchaseTransaction, useFinancialAccounts,
  usePurchaseTransactions,
} from './queries'
import './payments.css'

/** The earlier payments' quick filters, after Moneybird's "Open or pending payments from this year". */
const views = { pending: 'open|pending_payment', paid: 'paid', cancelled: 'cancelled', all: 'all' } as const
type View = keyof typeof views

/**
 * Moneybird's payments (/purchase_transaction_batches): the transfers set
 * up on purchase invoices and receipts that are still to send, gathered in
 * a batch to download as one SEPA credit transfer file; and the earlier
 * payments with how they were sent and their status.
 */
export function PaymentsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search] = useSearchParams()
  const administration = useAdministration(administrationId)
  const accounts = useFinancialAccounts(administrationId)
  const view: View = (Object.keys(views) as View[]).includes(search.get('view') as View) ? (search.get('view') as View) : 'pending'
  const toSend = usePurchaseTransactions(administrationId, 'state:open,unbatched:true')
  const earlier = usePurchaseTransactions(administrationId, `state:${views[view]},period:this_year,unbatched:false`)
  const batch = useCreatePaymentBatch(administrationId)
  const remove = useDeletePurchaseTransaction(administrationId)
  const [info, setInfo] = useState(false)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [made, setMade] = useState<string[]>([])
  const infoId = useId()

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const base = `/${administrationId}`
  const money = (v: string | null | undefined) => formatAmount(v, administration.data?.currency ?? 'EUR', i18n.language)
  const accountName = (id: string | undefined) => {
    const a = accounts.data?.find((acc) => acc.id === id)
    return a ? (a.identifier ? groupIBAN(a.identifier) : (a.name ?? '')) : ''
  }
  const open = toSend.data ?? []
  const chosen = open.filter((p) => picked.has(p.id ?? ''))
  const toggle = (id: string) => {
    const next = new Set(picked)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setPicked(next)
  }
  // A batch is one account's: the payments chosen are batched per account they are paid from.
  const makeBatches = async () => {
    const byAccount = new Map<string, string[]>()
    for (const p of chosen.length > 0 ? chosen : open) {
      const ids = byAccount.get(p.financial_account_id ?? '') ?? []
      ids.push(p.id ?? '')
      byAccount.set(p.financial_account_id ?? '', ids)
    }
    const ids: string[] = []
    for (const [account, list] of byAccount) {
      const b = await batch.mutateAsync({ financial_account_id: account, purchase_transaction_ids: list })
      if (b.id) ids.push(b.id)
    }
    setPicked(new Set())
    setMade(ids)
  }

  return (
    <Page
      wide
      title={t('bank.payments.title')}
      actions={
        <div className="accounts-actions">
          <button
            type="button"
            className={`btn btn--secondary btn--icon${info ? ' is-active' : ''}`}
            aria-label={t('bank.accounts.moreInfo')}
            title={t('bank.accounts.moreInfo')}
            aria-expanded={info}
            aria-controls={infoId}
            onClick={() => setInfo(!info)}
          >
            <InfoIcon />
          </button>
          <ActionMenu label={t('bank.accounts.more')} iconOnly icon={<ListIcon kind="more" />} items={[{ label: t('bank.settings'), to: `${base}/settings/bank` }]} />
        </div>
      }
    >
      {info && (
        <GettingStarted
          id={infoId}
          title={t('bank.payments.intro.title')}
          text={t('bank.payments.intro.text')}
          steps={[
            { icon: 'plus', title: t('bank.payments.intro.add'), text: t('bank.payments.intro.addText') },
            { icon: 'send', title: t('bank.payments.intro.batch'), text: t('bank.payments.intro.batchText') },
            { icon: 'flow', title: t('bank.payments.intro.view'), text: t('bank.payments.intro.viewText') },
          ]}
        />
      )}

      <section className="payments-section" aria-labelledby={`${infoId}-send`}>
        <div className="payments-section__bar">
          <h2 id={`${infoId}-send`} className="payments-section__title">{t('bank.payments.toSend')}</h2>
          {open.length > 0 && (
            <button type="button" className="btn btn--primary btn--with-icon" disabled={batch.isPending} onClick={() => void makeBatches()}>
              <ListIcon kind="send" />
              <span>{chosen.length > 0 ? t('bank.payments.batchChosen', { count: chosen.length }) : t('bank.payments.batchAll')}</span>
            </button>
          )}
        </div>
        <Problems messages={[...formErrors(batch.error, {}).general, ...formErrors(remove.error, {}).general]} />
        {made.length > 0 && (
          <p className="notice" role="status">
            {t('bank.payments.batchMade', { count: made.length })}{' '}
            {made.map((id, i) => (
              <span key={id}>
                {i > 0 && ', '}
                <a href={paymentBatchURL(administrationId, id)} download>{t('bank.payments.download')}</a>
              </span>
            ))}
          </p>
        )}
        {!toSend.data ? (
          <Loading />
        ) : open.length === 0 ? (
          <p className="payments-section__empty">{t('bank.payments.toSendEmpty')}</p>
        ) : (
          <ul className="payments-to-send" aria-label={t('bank.payments.toSend')}>
            {open.map((p) => (
              <li key={p.id} className="payments-to-send__row">
                <input type="checkbox" aria-label={t('bank.payments.choose', { name: p.sepa_iban_account_name || p.description })} checked={picked.has(p.id ?? '')} onChange={() => toggle(p.id ?? '')} />
                <div className="payments-to-send__main">
                  <span className="payments-to-send__name">{p.sepa_iban_account_name || p.contact?.company_name || t('bank.noDescription')}</span>
                  <span className="payments-to-send__sub">{[groupIBAN(p.sepa_iban ?? ''), p.description].filter(Boolean).join(' · ')}</span>
                  <span className="payments-to-send__sub">{t('bank.payments.from', { account: accountName(p.financial_account_id) })}</span>
                </div>
                <span className="payments-to-send__amount">{money(p.amount)}</span>
                <button
                  type="button"
                  className="btn btn--secondary btn--icon btn--small"
                  aria-label={t('bank.payments.remove', { name: p.sepa_iban_account_name || p.description })}
                  title={t('bank.payments.removeShort')}
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(p.id ?? '')}
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="payments-section" aria-labelledby={`${infoId}-earlier`}>
        <div className="payments-section__bar">
          <h2 id={`${infoId}-earlier`} className="payments-section__title">{t('bank.payments.earlier')}</h2>
          <FilterPanel
            label={t(`bank.payments.views.${view}`)}
            title={t('bank.payments.filter')}
            quick={(Object.keys(views) as View[]).map((v) => ({
              label: t(`bank.payments.views.${v}`), to: v === 'pending' ? `${base}/purchase_transaction_batches` : `${base}/purchase_transaction_batches?view=${v}`, current: v === view,
            }))}
          />
        </div>
        <EarlierPayments payments={earlier.data} money={money} accountName={accountName} administrationId={administrationId} />
      </section>
    </Page>
  )
}

type EarlierProps = {
  payments: PurchaseTransaction[] | undefined
  money: (v: string | null | undefined) => string
  accountName: (id: string | undefined) => string
  administrationId: string
}

/** Moneybird's table of earlier payments: reference, account, date, method, amount and status. */
function EarlierPayments({ payments, money, accountName, administrationId }: EarlierProps) {
  const { t } = useTranslation()
  if (!payments) {
    return <Loading />
  }
  return (
    <div>
      <table className="data-table payments-table" aria-label={t('bank.payments.earlier')}>
        <thead>
          <tr>
            <th scope="col">{t('bank.payments.reference')}</th>
            <th scope="col">{t('bank.account')}</th>
            <th scope="col">{t('bank.payments.date')}</th>
            <th scope="col">{t('bank.payments.method')}</th>
            <th scope="col" className="payments-table__amount">{t('bank.amount')}</th>
            <th scope="col">{t('bank.payments.status')}</th>
          </tr>
        </thead>
        <tbody>
          {payments.length === 0 ? (
            <tr>
              <td colSpan={6} className="payments-table__empty">{t('bank.payments.earlierEmpty')}</td>
            </tr>
          ) : (
            payments.map((p) => (
              <tr key={p.id}>
                <td>
                  {p.payable_id ? <Link to={`/${administrationId}/documents/${p.payable_id}`}>{p.description || p.end_to_end_id}</Link> : p.description || p.end_to_end_id}
                  {p.sepa_iban_account_name && <span className="hint">{p.sepa_iban_account_name}</span>}
                </td>
                <td>{accountName(p.financial_account_id)}</td>
                <td>{p.date ?? ''}</td>
                <td>
                  {t('bank.payments.sepa')}
                  {p.purchase_transaction_batch_id && (
                    <> · <a href={paymentBatchURL(administrationId, String(p.purchase_transaction_batch_id))} download>{t('bank.payments.batchFile')}</a></>
                  )}
                </td>
                <td className="payments-table__amount">{money(p.amount)}</td>
                <td><span className={`badge badge--${p.state}`}>{t(`bank.payments.states.${p.state}`, { defaultValue: p.state })}</span></td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
