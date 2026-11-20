import { ActionLink } from '../layout/ActionLink'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { CaretIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { useListFromTemplate, useTemplates } from './queries'
import './tasklists.css'

/**
 * Moneybird's "Add from template": the administration's templates, one of
 * which makes a task list for the period before the current one
 * (/task_lists/lists/templates_selection).
 */
export function TemplateSelectionPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const templates = useTemplates(administrationId)
  const make = useListFromTemplate(administrationId)
  const base = `/${administrationId}/task_lists`
  return (
    <Page wide title={t('taskLists.chooseTemplate')}>
      <Problems messages={formErrors(make.error, {}).general} />
      {!templates.data ? (
        templates.error ? <p className="empty">{t('taskLists.loadError')}</p> : <Loading />
      ) : templates.data.length === 0 ? (
        <p className="empty">
          {t('taskLists.noTemplates')} <Link to={`${base}/templates/new`}>{t('taskLists.addTemplate')}</Link>
        </p>
      ) : (
        <ul className="choice-list" aria-label={t('taskLists.chooseTemplate')}>
          {templates.data.map((tpl) => (
            <li key={tpl.id}>
              <button
                type="button"
                className="choice-list__item"
                disabled={make.isPending}
                onClick={() => make.mutate(tpl.id ?? '', { onSuccess: (made) => navigate(`${base}/lists/${made.id}`) })}
              >
                <span className="choice-list__text">
                  <span className="choice-list__title">{tpl.name}</span>
                  {tpl.description && <span className="choice-list__help">{tpl.description}</span>}
                </span>
                <CaretIcon />
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="choice-list__foot">
        <ActionLink className="btn btn--secondary" to={`${base}/lists`}>{t('common.cancel')}</ActionLink>
      </div>
    </Page>
  )
}
