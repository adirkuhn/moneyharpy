import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, FormSection, formErrors, SelectInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { PermissionFields } from './PermissionFields'
import { type Permission, type User, type UserType, userTypes, useRemoveUser, useUpdateUser, useUsers } from './queries'

/** Changes a user's access (/users/:userId). Owners only. */
export function UserFormPage() {
  const { administrationId = '', userId = '' } = useParams()
  const users = useUsers(administrationId)
  if (!users.data) {
    return <Loading />
  }
  const u = users.data.find((x) => x.id === userId)
  if (!u) {
    return <NotFoundPage />
  }
  return <UserForm key={u.id} administrationId={administrationId} user={u} />
}

function UserForm({ administrationId, user: u }: { administrationId: string; user: User }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useUpdateUser(administrationId, u.id ?? '')
  const remove = useRemoveUser(administrationId)
  const [form, setForm] = useState({
    user_type: (u.user_type ?? 'employee') as UserType,
    permissions: (u.permissions ?? []) as Permission[],
    active: u.active,
  })
  const back = `/${administrationId}/settings/administration#users`
  const errors = formErrors(save.error ?? remove.error, { user_type: t('management.access'), permissions: t('management.permissions') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(form, { onSuccess: () => navigate(back) })
  }
  const removeAction = (
    <ConfirmDelete label={t('management.removeUser')} confirmLabel={t('management.confirmRemoveUser')} disabled={remove.isPending} onConfirm={() => remove.mutate(u.id ?? '', { onSuccess: () => navigate(back) })} />
  )
  return (
    <Page title={u.name ?? ''} subtitle={u.email ?? ''} back={{ to: back, label: t('management.back') }} actions={removeAction}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <SelectInput label={t('management.access')} width="medium" value={form.user_type} errors={errors.field('user_type')} onChange={(e) => setForm({ ...form, user_type: e.target.value as UserType })}>
            {userTypes.map((type) => <option key={type} value={type}>{t(`management.types.${type}`)}</option>)}
          </SelectInput>
          <CheckboxInput label={t('bookkeeping.active')} help={t('management.activeHelp')} checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} />
        </FormSection>
        <PermissionFields type={form.user_type} value={form.permissions} onChange={(permissions) => setForm({ ...form, permissions })} />
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
