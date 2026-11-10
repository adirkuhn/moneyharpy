import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { balanceSheetTypes, type LedgerAccount, profitAndLossTypes, useLedgerAccounts } from '../bookkeeping/queries'
import { formatAmount } from '../banking/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { byAccount, reportPeriods, useGeneralLedger } from './queries'

/** Every category's balance at the start, its debits and credits, and its balance at the end of a period. */
export function TrialBalancePage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const period = (reportPeriods as readonly string[]).includes(search.get('period') ?? '') ? search.get('period')! : 'this_month'
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const ledger = useGeneralLedger(administrationId, period)
  const currency = administration.data?.currency ?? 'EUR'
  const money = (n: number) => formatAmount(n, currency, i18n.language)

  const filters = (
    <div className="filter-bar" role="search" aria-label={t('journal.filters')}>
      <label className="filter-bar__field">
        <span>{t('journal.period')}</span>
        <select value={period} onChange={(e) => setSearch({ period: e.target.value }, { replace: true })}>
          {reportPeriods.map((p) => <option key={p} value={p}>{t(`journal.periods.${p}`)}</option>)}
        </select>
      </label>
    </div>
  )
  if (!ledger.data || !accounts.data) {
    return <Page title={t('journal.trialBalance')}>{filters}{ledger.error ? <p className="empty">{t('journal.loadError')}</p> : <Loading />}</Page>
  }
  const debit = byAccount(ledger.data.debit_sums)
  const credit = byAccount(ledger.data.credit_sums)
  const start = byAccount(ledger.data.start_balance)
  const end = byAccount(ledger.data.final_balance)
  const shown = new Set([...debit.keys(), ...credit.keys(), ...start.keys(), ...end.keys()])
  const groups = [...balanceSheetTypes, ...profitAndLossTypes]
    .map((type) => ({ type, rows: accounts.data.filter((a) => a.account_type === type && shown.has(a.id ?? '')).sort(byCode) }))
    .filter((g) => g.rows.length > 0)
  const sum = (m: Map<string, number>) => [...m.values()].reduce((a, b) => a + b, 0)

  return (
    <Page title={t('journal.trialBalance')} subtitle={t('journal.trialBalanceSubtitle')}>
      {filters}
      {groups.length === 0 ? (
        <p className="empty">{t('journal.nothingBooked')}</p>
      ) : (
        <table className="data-table ledger-report" aria-label={t('journal.trialBalance')}>
          <thead>
            <tr>
              <th scope="col">{t('journal.category')}</th>
              <th scope="col" className="ledger-report__amount data-table__extra">{t('journal.startBalance')}</th>
              <th scope="col" className="ledger-report__amount">{t('journal.debit')}</th>
              <th scope="col" className="ledger-report__amount">{t('journal.credit')}</th>
              <th scope="col" className="ledger-report__amount">{t('journal.endBalance')}</th>
            </tr>
          </thead>
          {groups.map((g) => (
            <tbody key={g.type}>
              <tr className="category-table__group">
                <th scope="rowgroup" colSpan={5}>{t(`bookkeeping.accountTypes.${g.type}`)}</th>
              </tr>
              {g.rows.map((a) => (
                <tr key={a.id}>
                  <td>
                    <Link to={`/${administrationId}/report/journal_entries?ledger_account_id=${a.id}&period=${period}`}>
                      {a.account_id && <span className="code-chip">{a.account_id}</span>}
                      {a.name}
                    </Link>
                  </td>
                  <td className="ledger-report__amount data-table__extra">{money(start.get(a.id ?? '') ?? 0)}</td>
                  <td className="ledger-report__amount">{money(debit.get(a.id ?? '') ?? 0)}</td>
                  <td className="ledger-report__amount">{money(credit.get(a.id ?? '') ?? 0)}</td>
                  <td className="ledger-report__amount">{money(end.get(a.id ?? '') ?? 0)}</td>
                </tr>
              ))}
            </tbody>
          ))}
          <tfoot>
            <tr className="ledger-report__total">
              <th scope="row">{t('journal.total')}</th>
              <td className="data-table__extra" />
              <td className="ledger-report__amount">{money(sum(debit))}</td>
              <td className="ledger-report__amount">{money(sum(credit))}</td>
              <td />
            </tr>
          </tfoot>
        </table>
      )}
    </Page>
  )
}

function byCode(a: LedgerAccount, b: LedgerAccount) {
  return (a.account_id ?? '').localeCompare(b.account_id ?? '') || (a.name ?? '').localeCompare(b.name ?? '')
}
