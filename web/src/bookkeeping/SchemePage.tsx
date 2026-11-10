import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { errorMessages } from '../api/client'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { balanceSheetTypes, type LedgerAccount, profitAndLossTypes, tree, useBookkeepingSettings, useLedgerAccounts, useUpdateScheme } from './queries'

/** Edits the chart scheme code of every category on one page. */
export function SchemePage() {
  const { administrationId = '' } = useParams()
  const settings = useBookkeepingSettings(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  if (!settings.data || !accounts.data) {
    return <Loading />
  }
  const scheme = settings.data.chart_schemes.find((s) => s.id === settings.data.chart_scheme)
  return <SchemeForm administrationId={administrationId} schemeName={scheme ? `${scheme.name} ${scheme.version}` : ''} accounts={accounts.data} />
}

function SchemeForm({ administrationId, schemeName, accounts }: { administrationId: string; schemeName: string; accounts: LedgerAccount[] }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useUpdateScheme(administrationId)
  const [codes, setCodes] = useState<Record<string, string>>(() => Object.fromEntries(accounts.map((a) => [a.id ?? '', a.taxonomy_item?.code ?? ''])))
  const back = `/${administrationId}/settings/bookkeeping#categories`
  const byID = new Map(accounts.map((a) => [a.id ?? '', a]))
  const labels: Record<string, string> = Object.fromEntries(accounts.map((a) => [`ledger_accounts.${a.id}`, a.name ?? '']))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const changed = Object.entries(codes)
      .filter(([id, code]) => (byID.get(id)?.taxonomy_item?.code ?? '') !== code)
      .map(([id, rgs_code]) => ({ id, rgs_code }))
    save.mutate(changed, { onSuccess: () => navigate(back) })
  }
  const order: string[] = [...profitAndLossTypes, ...balanceSheetTypes]
  const ordered = tree(accounts).sort((a, b) => order.indexOf(a.account.account_type ?? '') - order.indexOf(b.account.account_type ?? ''))

  return (
    <Page title={t('bookkeeping.editScheme')} subtitle={schemeName} back={{ to: back, label: t('bookkeeping.back') }}>
      <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate>
        <Problems messages={errorMessages(save.error, labels)} />
        <table className="data-table" aria-label={t('bookkeeping.editScheme')}>
          <thead>
            <tr>
              <th scope="col">{t('bookkeeping.categoryName')}</th>
              <th scope="col">{t('bookkeeping.schemeCode')}</th>
            </tr>
          </thead>
          <tbody>
            {ordered.map(({ account: a, depth }) => (
              <tr key={a.id}>
                <td>
                  <span style={{ paddingInlineStart: `${depth * 1.25}rem` }}>
                    {a.account_id && <span className="code-chip">{a.account_id}</span>} {a.name}
                  </span>
                </td>
                <td>
                  <input
                    type="text"
                    className="scheme-input"
                    aria-label={t('bookkeeping.schemeCodeFor', { name: a.name })}
                    maxLength={32}
                    spellCheck={false}
                    value={codes[a.id ?? ''] ?? ''}
                    onChange={(e) => setCodes({ ...codes, [a.id ?? '']: e.target.value.trim() })}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
