import { ActionLink } from '../layout/ActionLink'
import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formErrors, FormNotice, FormSection, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { useEmptyAdministration } from './queries'

/**
 * Empties the administration after typing its name: its data goes, its
 * setup stays, so it can start over, such as before importing it again.
 */
export function EmptyPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const empty = useEmptyAdministration(administrationId)
  const [name, setName] = useState('')
  if (!administration.data) {
    return <Loading />
  }
  const a = administration.data
  const back = `/${administrationId}/settings/administration#leave-or-delete`
  const errors = formErrors(empty.error, { name: t('management.typeName') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    empty.mutate(name, { onSuccess: () => navigate(`/${administrationId}/feed`, { replace: true }) })
  }
  return (
    <Page title={t('management.emptyTitle', { name: a.name })} back={{ to: back, label: t('management.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate aria-label={t('management.emptyTitle', { name: a.name })}>
        <FormNotice>{t('management.emptyWarning')}</FormNotice>
        <div className="empty-lists">
          <div>
            <h2 className="empty-lists__title">{t('management.emptyGoes')}</h2>
            <p className="hint">{t('management.emptyGoesList')}</p>
          </div>
          <div>
            <h2 className="empty-lists__title">{t('management.emptyStays')}</h2>
            <p className="hint">{t('management.emptyStaysList')}</p>
          </div>
        </div>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={t('management.typeName')} help={t('management.typeNameHelp', { name: a.name })} autoComplete="off" spellCheck={false} value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--danger" disabled={empty.isPending || empty.isSuccess || name.trim() !== a.name}>{t('management.emptyForGood')}</button>
          <ActionLink className="btn btn--secondary" to={back}>{t('common.cancel')}</ActionLink>
        </div>
      </form>
    </Page>
  )
}
