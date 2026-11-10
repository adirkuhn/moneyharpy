import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { formErrors, FormNotice, FormSection, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { useDeleteAdministration } from './queries'

/** Deletes the administration for good, after typing its name. */
export function DeletePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const remove = useDeleteAdministration(administrationId)
  const [name, setName] = useState('')
  if (!administration.data) {
    return <Loading />
  }
  const a = administration.data
  const back = `/${administrationId}/settings/administration#leave-or-delete`
  const errors = formErrors(remove.error, { name: t('management.typeName') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    remove.mutate(name, { onSuccess: () => navigate('/administrations', { replace: true }) })
  }
  return (
    <Page title={t('management.deleteTitle', { name: a.name })} back={{ to: back, label: t('management.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <FormNotice>{t('management.deleteWarning')}</FormNotice>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={t('management.typeName')} help={t('management.typeNameHelp', { name: a.name })} autoComplete="off" spellCheck={false} value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--danger" disabled={remove.isPending || name.trim() !== a.name}>{t('management.deleteForGood')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
