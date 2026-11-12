import { type FormEvent, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { errorMessages } from '../api/client'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { useCategoryFlow } from './flow'
import { type AccountType, balanceSheetTypes, type LedgerAccount, profitAndLossTypes, type Template, useApplyTemplate, useLedgerAccounts, useTemplates } from './queries'
import './categories.css'

type TemplateAccount = Template['ledger_accounts'][number]

/**
 * Moneybird's "Add category from <theme>"
 * (/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=<key>):
 * the theme's categories by part of the books, those you have already ticked
 * and greyed out. Saving adds the others, then goes back.
 */
export function ThemePage() {
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const templates = useTemplates(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  if (!templates.data || !accounts.data) {
    return <Loading />
  }
  const template = templates.data.find((tpl) => tpl.key === params.get('ledger_account_set'))
  if (!template) {
    return <NotFoundPage />
  }
  return <Theme administrationId={administrationId} template={template} accounts={accounts.data} />
}

function Theme({ administrationId, template, accounts }: { administrationId: string; template: Template; accounts: LedgerAccount[] }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const flow = useCategoryFlow(administrationId)
  const apply = useApplyTemplate(administrationId)
  // The contract says whether the whole theme is there; a single category counts as there when one of its type has its number.
  const exists = (a: TemplateAccount) =>
    template.applied || accounts.some((o) => o.account_type === a.account_type && !!a.account_id && o.account_id === a.account_id)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (template.ledger_accounts.every(exists)) {
      navigate(flow.returnTo)
      return
    }
    apply.mutate(template.key, { onSuccess: () => navigate(flow.returnTo) })
  }
  const sides = [
    { title: t('bookkeeping.theme.balanceSheet'), types: balanceSheetTypes },
    { title: t('bookkeeping.theme.profitAndLoss'), types: profitAndLossTypes },
  ]
  return (
    <Page title={flow.forInvoice ? t('bookkeeping.theme.titleFor', { name: template.name }) : t('bookkeeping.theme.title', { name: template.name })}>
      <form className="category-card category-theme" onSubmit={submit} noValidate>
        <p>{template.description}</p>
        <Problems messages={errorMessages(apply.error, {})} />
        {sides.map((side) => (
          <Side key={side.title} title={side.title} types={side.types} accounts={template.ledger_accounts} exists={exists} />
        ))}
        <p className="hint">{t('bookkeeping.theme.together')}</p>
        <div className="category-card__actions category-card__actions--center">
          <button className="btn btn--primary" disabled={apply.isPending}>{flow.forInvoice ? t('bookkeeping.theme.saveBack') : t('settings.save')}</button>
          <Link className="btn btn--secondary" to={flow.returnTo}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}

type SideProps = { title: string; types: AccountType[]; accounts: TemplateAccount[]; exists: (a: TemplateAccount) => boolean }

/** One part of the books, with a row per type: its name on the left and its categories on the right. */
function Side({ title, types, accounts, exists }: SideProps) {
  const { t } = useTranslation()
  const heading = useId()
  const existsId = useId()
  const shown = types.filter((type) => accounts.some((a) => a.account_type === type))
  if (shown.length === 0) {
    return null
  }
  return (
    <fieldset className="category-theme__side" aria-labelledby={heading}>
      <h3 id={heading} className="category-theme__side-title">{title}</h3>
      <span id={existsId} hidden>{t('bookkeeping.theme.exists')}</span>
      {shown.map((type) => (
        <div key={type} className="category-theme__type">
          <h4 className="category-theme__type-title">{t(`bookkeeping.accountTypes.${type}`)}</h4>
          <ul className="category-theme__list">
            {accounts.filter((a) => a.account_type === type).map((a) => {
              const there = exists(a)
              return (
                <li key={`${a.account_id}${a.name}`}>
                  <label className={`category-theme__account${there ? ' is-there' : ''}`} title={there ? t('bookkeeping.theme.exists') : undefined}>
                    <input type="checkbox" checked disabled aria-describedby={there ? existsId : undefined} readOnly />
                    <span>{a.name}</span>
                    {a.account_id && <span className="code-chip">{a.account_id}</span>}
                  </label>
                </li>
              )
            })}
          </ul>
        </div>
      ))}
    </fieldset>
  )
}
