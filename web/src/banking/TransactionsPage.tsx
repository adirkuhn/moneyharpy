import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { formatAmount, useFinancialAccounts, useMutations } from './queries'

const states = ['unprocessed', 'processed', 'all'] as const
const periods = ['this_month', 'prev_month', 'this_quarter', 'prev_quarter', 'this_year', 'prev_year'] as const

/** The administration's transactions, filtered by account, state and period. */
export function TransactionsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const administration = useAdministration(administrationId)
  const accounts = useFinancialAccounts(administrationId)
  const state = (states as readonly string[]).includes(search.get('state') ?? '') ? search.get('state')! : 'unprocessed'
  const period = (periods as readonly string[]).includes(search.get('period') ?? '') ? search.get('period')! : 'this_year'
  const account = search.get('account') ?? ''
  const filter = [`period:${period}`, `state:${state}`, ...(account ? [`financial_account_id:${account}`] : [])].join(',')
  const mutations = useMutations(administrationId, filter)

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const set = (key: string, value: string) => {
    const next = new URLSearchParams(search)
    if (value) {
      next.set(key, value)
    } else {
      next.delete(key)
    }
    setSearch(next, { replace: true })
  }
  const names = new Map(accounts.data?.map((a) => [a.id, a.name]))
  const currency = administration.data.currency
  const date = (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })

  return (
    <Page
      title={t('bank.transactions')}
      subtitle={t('bank.transactionsSubtitle')}
      actions={<Link className="btn btn--secondary" to={`/${administrationId}/settings/bank`}>{t('bank.settings')}</Link>}
    >
      <div className="filter-bar" role="search" aria-label={t('bank.filters')}>
        <fieldset className="segmented">
          <legend className="sr-only">{t('bank.state')}</legend>
          <div className="segmented__options">
            {states.map((s) => (
              <label key={s} className={`segmented__option${state === s ? ' is-checked' : ''}`}>
                <input type="radio" name="state" value={s} checked={state === s} onChange={() => set('state', s)} />
                {t(`bank.states.${s}`)}
              </label>
            ))}
          </div>
        </fieldset>
        <label className="filter-bar__field">
          <span>{t('bank.account')}</span>
          <select value={account} onChange={(e) => set('account', e.target.value)}>
            <option value="">{t('bank.allAccounts')}</option>
            {accounts.data?.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </select>
        </label>
        <label className="filter-bar__field">
          <span>{t('bank.period')}</span>
          <select value={period} onChange={(e) => set('period', e.target.value)}>
            {periods.map((p) => <option key={p} value={p}>{t(`bank.periods.${p}`)}</option>)}
          </select>
        </label>
      </div>
      {!mutations.data ? (
        <Loading />
      ) : mutations.data.length === 0 ? (
        <p className="empty">{state === 'unprocessed' ? t('bank.nothingToBook') : t('bank.noTransactions')}</p>
      ) : (
        <table className="data-table transactions" aria-label={t('bank.transactions')}>
          <thead>
            <tr>
              <th scope="col" className="transactions__date">{t('bank.date')}</th>
              <th scope="col">{t('bank.description')}</th>
              <th scope="col" className="transactions__amount">{t('bank.amount')}</th>
              <th scope="col" className="transactions__amount data-table__extra">{t('bank.open')}</th>
            </tr>
          </thead>
          <tbody>
            {mutations.data.map((m) => (
              <tr key={m.id}>
                <td className="transactions__date">{date(m.date ?? '')}</td>
                <td>
                  <Link to={`/${administrationId}/financial_mutations/${m.id}`}>{m.contra_account_name || m.message || t('bank.noDescription')}</Link>
                  {m.contra_account_name && m.message && <p className="hint">{m.message}</p>}
                  {!account && <p className="hint">{names.get(m.financial_account_id ?? '')}</p>}
                </td>
                <td className={`transactions__amount${Number(m.amount) < 0 ? ' is-out' : ' is-in'}`}>{formatAmount(m.amount, m.currency ?? currency, i18n.language)}</td>
                <td className="transactions__amount data-table__extra">
                  {m.state === 'processed' ? <span className="ledger__state">{t('bank.booked')}</span> : formatAmount(m.amount_open, m.currency ?? currency, i18n.language)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Page>
  )
}
