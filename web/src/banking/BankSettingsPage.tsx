import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { SwitchRow } from '../invoicing/Switch'
import { Block } from '../settings/Block'
import { type FinancialAccount, useBankSettings, useBookingRules, useFinancialAccounts, useReorderFinancialAccounts, useSaveFinancialAccount, useUpdateBankSettings } from './queries'

/** Settings > Bank, block by block like Moneybird. */
export function BankSettingsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const settings = useBankSettings(administrationId)
  const update = useUpdateBankSettings(administrationId)
  const { hash } = useLocation()
  const ready = Boolean(settings.data && administration.data)

  useEffect(() => {
    if (hash && ready) {
      document.getElementById(hash.slice(1))?.scrollIntoView?.({ block: 'start' })
    }
  }, [hash, ready])

  if (!settings.data || !administration.data) {
    return <Loading />
  }
  return (
    <Page title={t('bank.title')} subtitle={t('bank.subtitle')}>
      <AccountsBlock administrationId={administrationId} />
      <Block id="bank-connections" title={t('bank.parts.connections')} subtitle={t('bank.connectionsSubtitle')}>
        <div className="card">
          <p className="card__title">{t('bank.statementFiles')}</p>
          <p className="hint">{t('bank.statementFilesHelp')}</p>
          <p className="card__footer">{t('bank.connectionsLater')}</p>
        </div>
      </Block>
      <Block id="automatic-booking" title={t('bank.parts.automatic')} subtitle={t('bank.automaticSubtitle')}>
        <div className="card">
          <SwitchRow label={t('bank.automatic')} help={t('bank.automaticHelp')} checked={settings.data.automatic_booking} disabled={update.isPending} onChange={(on) => update.mutate(on)} />
        </div>
      </Block>
      <RulesBlock administrationId={administrationId} />
      <Block id="payment-cards" title={t('bank.parts.cards')} subtitle={t('bank.cardsSubtitle')}>
        <p className="empty">{t('bank.cardsUnavailable')}</p>
      </Block>
      <Block id="smart-transfers" title={t('bank.parts.transfers')} subtitle={t('bank.transfersSubtitle')}>
        <p className="empty">{t('bank.transfersUnavailable')}</p>
      </Block>
    </Page>
  )
}

function AccountsBlock({ administrationId }: { administrationId: string }) {
  const { t, i18n } = useTranslation()
  const accounts = useFinancialAccounts(administrationId)
  const reorder = useReorderFinancialAccounts(administrationId)
  const [showInactive, setShowInactive] = useState(false)
  const all = accounts.data ?? []
  const inactive = all.filter((a) => !a.active).length
  const shown = all.filter((a) => a.active || showInactive)
  const move = (a: FinancialAccount, by: number) => {
    const ids = all.map((x) => x.id ?? '')
    const from = ids.indexOf(a.id ?? '')
    const to = from + by
    if (to < 0 || to >= ids.length) {
      return
    }
    const [moved] = ids.splice(from, 1)
    ids.splice(to, 0, moved ?? '')
    reorder.mutate(ids)
  }
  const updated = (a: FinancialAccount) => (a.last_import_at ? new Date(a.last_import_at).toLocaleDateString(i18n.language, { dateStyle: 'medium' }) : '-')

  return (
    <Block
      id="accounts"
      title={t('bank.parts.accounts')}
      subtitle={t('bank.accountsSubtitle')}
      action={<Link className="btn btn--accent" to={`/${administrationId}/financial_accounts/new`}>{t('settings.add')}</Link>}
    >
      <table className="data-table" aria-label={t('bank.accountsTable')}>
        <thead>
          <tr>
            <th scope="col">{t('bank.accountName')}</th>
            <th scope="col" className="data-table__extra">{t('bank.connection')}</th>
            <th scope="col" className="data-table__extra">{t('bank.updated')}</th>
            <th scope="col"><span className="sr-only">{t('bookkeeping.actions')}</span></th>
          </tr>
        </thead>
        <tbody>
          {shown.map((a) => (
            <tr key={a.id} className={a.active ? undefined : 'is-inactive'}>
              <td>
                <Link to={`/${administrationId}/financial_accounts/${a.id}`}>{a.name}</Link>
                {a.identifier && <span className="hint"> · {a.identifier}</span>}
                {!a.active && <span className="hint"> · {t('bookkeeping.inactive')}</span>}
              </td>
              <td className="data-table__extra">{a.provider ?? t('bank.files')}</td>
              <td className="data-table__extra">{updated(a)}</td>
              <td className="data-table__actions">
                {a.active ? (
                  <>
                    <button type="button" className="btn btn--secondary btn--small" disabled={reorder.isPending || all[0]?.id === a.id} aria-label={t('bank.moveUp', { name: a.name })} onClick={() => move(a, -1)}>↑</button>
                    <button type="button" className="btn btn--secondary btn--small" disabled={reorder.isPending || all[all.length - 1]?.id === a.id} aria-label={t('bank.moveDown', { name: a.name })} onClick={() => move(a, 1)}>↓</button>
                  </>
                ) : (
                  <Reactivate administrationId={administrationId} account={a} />
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {inactive > 0 && (
        <button type="button" className="icon-link icon-link--button" onClick={() => setShowInactive(!showInactive)}>
          {showInactive ? t('bank.hideInactive') : t('bank.showInactive', { count: inactive })}
        </button>
      )}
    </Block>
  )
}

function Reactivate({ administrationId, account }: { administrationId: string; account: FinancialAccount }) {
  const { t } = useTranslation()
  const save = useSaveFinancialAccount(administrationId, account.id)
  return (
    <button type="button" className="btn btn--secondary btn--small" disabled={save.isPending} aria-label={t('bookkeeping.reactivateName', { name: account.name })} onClick={() => save.mutate({ active: true })}>
      {t('bookkeeping.reactivate')}
    </button>
  )
}

function RulesBlock({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const rules = useBookingRules(administrationId)
  return (
    <Block
      id="booking-rules"
      title={t('bank.parts.rules')}
      subtitle={t('bank.rulesSubtitle')}
      action={<Link className="btn btn--accent" to={`/${administrationId}/booking_rules/new`}>{t('settings.add')}</Link>}
    >
      {rules.data?.length ? (
        <table className="data-table" aria-label={t('bank.parts.rules')}>
          <thead>
            <tr>
              <th scope="col">{t('bank.ruleName')}</th>
              <th scope="col" className="data-table__extra">{t('bank.ruleSummary')}</th>
            </tr>
          </thead>
          <tbody>
            {rules.data.map((r) => (
              <tr key={r.id} className={r.active ? undefined : 'is-inactive'}>
                <td>
                  <Link to={`/${administrationId}/booking_rules/${r.id}`}>{r.name}</Link>
                  {!r.active && <span className="hint"> · {t('bookkeeping.inactive')}</span>}
                </td>
                <td className="data-table__extra">
                  {r.conditions.map((c) => `${t(`bank.fields.${c.field}`)} ${t(`bank.operators.${c.operator}`)} "${c.value}"`).join(r.match_all ? ` ${t('bank.and')} ` : ` ${t('bank.or')} `)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="empty">{t('bank.noRules')}</p>
      )}
    </Block>
  )
}
