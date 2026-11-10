import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { errorMessages } from '../api/client'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { useApplyTemplate, useTemplates } from './queries'

/** Groups of categories for common situations, such as a company car, added in one go. */
export function TemplatesPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const templates = useTemplates(administrationId)
  const apply = useApplyTemplate(administrationId)
  const back = `/${administrationId}/settings/bookkeeping#categories`
  if (!templates.data) {
    return <Loading />
  }
  return (
    <Page title={t('bookkeeping.templates')} subtitle={t('bookkeeping.templatesSubtitle')} back={{ to: back, label: t('bookkeeping.back') }}>
      <Problems messages={errorMessages(apply.error, {})} />
      {templates.data.length === 0 && <p className="empty">{t('bookkeeping.noTemplates')}</p>}
      <div className="card-stack">
        {templates.data.map((tpl) => (
          <section key={tpl.key} className="card template" aria-label={tpl.name}>
            <div className="switch-row">
              <div>
                <h2 className="card__title">{tpl.name}</h2>
                <p className="hint">{tpl.description}</p>
              </div>
              {tpl.applied ? (
                <span className="ledger__state">{t('bookkeeping.added')}</span>
              ) : (
                <button type="button" className="btn btn--accent btn--small" disabled={apply.isPending} aria-label={t('bookkeeping.addTemplate', { name: tpl.name })} onClick={() => apply.mutate(tpl.key)}>
                  {t('settings.add')}
                </button>
              )}
            </div>
            <ul className="template__accounts">
              {tpl.ledger_accounts.map((a) => (
                <li key={`${a.account_type}${a.name}`}>
                  {a.account_id && <span className="code-chip">{a.account_id}</span>} {a.name}
                  <span className="hint"> · {t(`bookkeeping.accountTypes.${a.account_type}`)}</span>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </Page>
  )
}
