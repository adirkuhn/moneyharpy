import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Menu } from '../layout/Menu'
import { Block } from '../settings/Block'
import { type AccountType, balanceSheetTypes, type LedgerAccount, profitAndLossTypes, tree, useDeleteLedgerAccount, useLedgerAccounts, useSaveLedgerAccount } from './queries'

/** The categories of the books: profit and loss first, then the balance sheet, grouped by type. */
export function CategoriesBlock({ administrationId, hasScheme }: { administrationId: string; hasScheme: boolean }) {
  const { t } = useTranslation()
  const accounts = useLedgerAccounts(administrationId)
  const base = `/${administrationId}`
  return (
    <Block
      id="categories"
      title={t('bookkeeping.parts.categories')}
      subtitle={t('bookkeeping.categoriesSubtitle')}
      action={
        <div className="block-actions">
          {hasScheme && <Link className="btn btn--secondary" to={`${base}/ledger_accounts/scheme`}>{t('bookkeeping.editScheme')}</Link>}
          <Menu label={t('settings.add')} buttonClassName="btn btn--accent" align="right">
            <ul className="menu__list">
              <li><Link to={`${base}/ledger_accounts/new`}>{t('bookkeeping.addCategory')}</Link></li>
              <li><Link to={`${base}/ledger_account_templates`}>{t('bookkeeping.addFromTemplate')}</Link></li>
            </ul>
          </Menu>
        </div>
      }
    >
      <CategoryTable administrationId={administrationId} title={t('bookkeeping.profitAndLoss')} types={profitAndLossTypes} accounts={accounts.data ?? []} />
      <CategoryTable administrationId={administrationId} title={t('bookkeeping.balanceSheet')} types={balanceSheetTypes} accounts={accounts.data ?? []} />
    </Block>
  )
}

type TableProps = { administrationId: string; title: string; types: AccountType[]; accounts: LedgerAccount[] }

function CategoryTable({ administrationId, title, types, accounts }: TableProps) {
  const { t } = useTranslation()
  const remove = useDeleteLedgerAccount(administrationId)
  return (
    <table className="data-table category-table" aria-label={title}>
      <thead>
        <tr>
          <th scope="col">{title}</th>
          <th scope="col" className="data-table__extra">{t('bookkeeping.schemeCode')}</th>
          <th scope="col"><span className="sr-only">{t('bookkeeping.actions')}</span></th>
        </tr>
      </thead>
      {types.map((type) => {
        const rows = tree(accounts.filter((a) => a.account_type === type))
        if (rows.length === 0) {
          return null
        }
        return (
          <tbody key={type}>
            <tr className="category-table__group">
              <th scope="rowgroup" colSpan={3}>{t(`bookkeeping.accountTypes.${type}`)}</th>
            </tr>
            {rows.map(({ account: a, depth }) => (
              <tr key={a.id} className={a.active ? undefined : 'is-inactive'}>
                <td>
                  <div className="category" style={{ paddingInlineStart: `${depth * 1.25}rem` }}>
                    {a.account_id && <span className="code-chip">{a.account_id}</span>}
                    <div className="category__text">
                      <Link to={`/${administrationId}/ledger_accounts/${a.id}`}>{a.name}</Link>
                      {!a.active && <span className="hint"> · {t('bookkeeping.inactive')}</span>}
                      {a.description && <p className="hint">{a.description}</p>}
                    </div>
                  </div>
                </td>
                <td className="data-table__extra">
                  {a.taxonomy_item ? <span title={a.taxonomy_item.name}>{a.taxonomy_item.code}</span> : <span className="hint">-</span>}
                </td>
                <td className="data-table__actions">
                  {a.active ? (
                    !a.system_role && (
                      <button type="button" className="btn btn--secondary btn--small" disabled={remove.isPending} aria-label={t('bookkeeping.deactivateName', { name: a.name })} onClick={() => remove.mutate(a.id ?? '')}>
                        {t('bookkeeping.deactivate')}
                      </button>
                    )
                  ) : (
                    <Reactivate administrationId={administrationId} account={a} />
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        )
      })}
    </table>
  )
}

function Reactivate({ administrationId, account }: { administrationId: string; account: LedgerAccount }) {
  const { t } = useTranslation()
  const save = useSaveLedgerAccount(administrationId, account.id)
  return (
    <button type="button" className="btn btn--secondary btn--small" disabled={save.isPending} aria-label={t('bookkeeping.reactivateName', { name: account.name })} onClick={() => save.mutate({ ledger_account: { active: true } })}>
      {t('bookkeeping.reactivate')}
    </button>
  )
}
