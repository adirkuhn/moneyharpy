import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { DocumentTable } from '../list/DocumentTable'
import { AdvancedFilter, FilterPanel, RadioFilter, SelectFilter } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import { allPeriods } from '../list/PeriodFilter'
import { isPeriod, parsePeriod, periodValue } from '../list/period'
import { PeriodPicker } from '../list/PeriodPicker'
import { formatAmount, type Mutation, useFinancialAccounts, useMutations } from './queries'

const states = ['unprocessed', 'processed', 'all'] as const
type State = (typeof states)[number]
const defaultPeriod = 'this_year'

type Filter = { state: State; period: string; account: string }

/** The administration's transactions, filtered by state, period and account (/financial_mutations). */
export function TransactionsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search, setSearch] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const administration = useAdministration(administrationId)
  const accounts = useFinancialAccounts(administrationId)
  const state: State = (states as readonly string[]).includes(search.get('state') ?? '') ? (search.get('state') as State) : 'unprocessed'
  const asked = search.get('period') ?? ''
  const period = asked === allPeriods || isPeriod(asked, today) ? asked : defaultPeriod
  const account = search.get('account') ?? ''
  // Without a period term the filter covers every period.
  const filter = [...(period === allPeriods ? [] : [`period:${period}`]), `state:${state}`, ...(account ? [`financial_account_id:${account}`] : [])].join(',')
  const mutations = useMutations(administrationId, filter)
  const [info, setInfo] = useState(false)
  const introId = useId()

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const base = `/${administrationId}/financial_mutations`
  const address = (f: Filter) => {
    const next = new URLSearchParams()
    if (f.state !== 'unprocessed') next.set('state', f.state)
    if (f.period !== defaultPeriod) next.set('period', f.period)
    if (f.account) next.set('account', f.account)
    return next
  }
  const apply = (f: Filter) => setSearch(address(f), { replace: true })
  const linkTo = (f: Filter) => {
    const query = address(f).toString()
    return query ? `${base}?${query}` : base
  }
  const custom = period !== defaultPeriod || account !== ''
  const names = new Map(accounts.data?.map((a) => [a.id, a.name]))
  const currency = administration.data.currency
  const money = (m: Mutation, v: string | null | undefined) => formatAmount(v, m.currency ?? currency, i18n.language)
  const date = (d: string) => new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const shown = mutations.data ?? []

  const filterPanel = (
    <FilterPanel
      label={t(`bank.views.${custom ? 'custom' : state}`)}
      title={t('bank.filters')}
      quick={states.map((s) => ({ label: t(`bank.views.${s}`), to: linkTo({ state: s, period: defaultPeriod, account: '' }), current: !custom && s === state }))}
    >
      <TransactionFilter filter={{ state, period, account }} today={today} accounts={(accounts.data ?? []).map((a) => ({ value: a.id ?? '', label: a.name ?? '' }))} onApply={apply} />
    </FilterPanel>
  )

  return (
    <Page
      wide
      title={t('bank.transactions')}
      lead={filterPanel}
      actions={
        <ListActions
          info={{ shown: info, onToggle: () => setInfo(!info), controls: introId }}
          exports={[]}
          primary={{ to: account ? `/${administrationId}/financial_accounts/${account}` : `/${administrationId}/settings/bank`, label: t('bank.importStatement') }}
          more={[
            { to: `/${administrationId}/settings/bank`, label: t('bank.settings') },
            { to: `/${administrationId}/booking_rules/new`, label: t('bank.newRule') },
          ]}
        />
      }
    >
      {info && (
        <GettingStarted
          id={introId}
          title={t('bank.intro.title')}
          text={t('bank.intro.text')}
          steps={[
            { icon: 'upload', title: t('bank.intro.import'), text: t('bank.intro.importText') },
            { icon: 'flow', title: t('bank.intro.rules'), text: t('bank.intro.rulesText') },
            { icon: 'coins', title: t('bank.intro.link'), text: t('bank.intro.linkText') },
          ]}
        />
      )}
      {!mutations.data ? (
        <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{state === 'unprocessed' ? t('bank.nothingToBook') : t('bank.noTransactions')}</p>
      ) : (
        <DocumentTable
          label={t('bank.transactions')}
          rows={shown}
          rowKey={(m) => m.id ?? ''}
          href={(m) => `${base}/${m.id}`}
          total={shown.length}
          perPage={Math.max(shown.length, 1)}
          columns={[
            { key: 'date', header: t('bank.date'), kind: 'date', cell: (m) => date(m.date ?? '') },
            { key: 'name', header: t('bank.contraName'), link: true, cell: (m) => m.contra_account_name || m.message || t('bank.noDescription') },
            { key: 'message', header: t('bank.description'), kind: 'extra', cell: (m) => (m.contra_account_name ? m.message : '') },
            ...(account ? [] : [{ key: 'account', header: t('bank.account'), kind: 'extra' as const, cell: (m: Mutation) => names.get(m.financial_account_id ?? '') }]),
            { key: 'amount', header: t('bank.amount'), kind: 'amount', cell: (m) => <span className={Number(m.amount) < 0 ? 'is-out' : 'is-in'}>{money(m, m.amount)}</span> },
            {
              key: 'open', header: t('bank.open'), kind: 'amount',
              cell: (m) => (m.state === 'processed' ? <span className="badge badge--paid">{t('bank.booked')}</span> : money(m, m.amount_open)),
            },
          ]}
        />
      )}
    </Page>
  )
}

type FilterProps = { filter: Filter; today: Date; accounts: { value: string; label: string }[]; onApply: (filter: Filter) => void }

/** The advanced filter of the transactions: a period, a state and an account. */
function TransactionFilter({ filter, today, accounts, onApply }: FilterProps) {
  const { t } = useTranslation()
  const [range, setRange] = useState(() => (filter.period === allPeriods ? null : parsePeriod(filter.period, today)))
  const [state, setState] = useState<State>(filter.state)
  const [account, setAccount] = useState(filter.account)
  return (
    <AdvancedFilter onApply={() => onApply({ state, account, period: range ? periodValue(range, today) : allPeriods })}>
      <PeriodPicker value={range} today={today} onChange={setRange} />
      <RadioFilter legend={t('bank.state')} value={state} onChange={(v) => setState(v as State)} options={states.map((s) => ({ value: s, label: t(`bank.states.${s}`) }))} />
      <SelectFilter label={t('bank.account')} value={account} onChange={setAccount} options={[{ value: '', label: t('bank.allAccounts') }, ...accounts]} />
    </AdvancedFilter>
  )
}
