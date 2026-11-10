import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { Page } from '../layout/Page'
import { PermissionFields } from './PermissionFields'
import { defaultPermissions, type Permission, type UserType, userTypes, useInvite } from './queries'

/** Invites someone by email (/user_invitations/new?type=employee). */
export function InvitePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const [search] = useSearchParams()
  const initial = (userTypes as string[]).includes(search.get('type') ?? '') ? (search.get('type') as UserType) : 'employee'
  const [form, setForm] = useState({ email: '', user_type: initial, permissions: defaultPermissions(initial) })
  const [link, setLink] = useState('')
  const invite = useInvite(administrationId)
  const back = `/${administrationId}/settings/administration#users`
  const labels = { email: t('management.email'), user_type: t('management.access'), permissions: t('management.permissions') }
  const errors = formErrors(invite.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    invite.mutate(form, {
      onSuccess: (inv) => (inv.email_sent ? navigate(back) : setLink(inv.accept_url)),
    })
  }
  if (link) {
    return (
      <Page title={t('management.inviteTitle')} back={{ to: back, label: t('management.back') }}>
        <div className="secret" role="status">
          <p>{t('management.shareLink')}</p>
          <code>{link}</code>
        </div>
        <Link className="btn btn--primary" to={back}>{t('management.done')}</Link>
      </Page>
    )
  }
  return (
    <Page title={t('management.inviteTitle')} subtitle={t('management.inviteSubtitle')} back={{ to: back, label: t('management.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.email} type="email" required autoComplete="off" maxLength={254} value={form.email} errors={errors.field('email')} onChange={(e) => setForm({ ...form, email: e.target.value })} />
          <SelectInput label={labels.user_type} width="medium" value={form.user_type} errors={errors.field('user_type')} onChange={(e) => {
            const user_type = e.target.value as UserType
            setForm({ ...form, user_type, permissions: defaultPermissions(user_type) })
          }}>
            {userTypes.map((type) => <option key={type} value={type}>{t(`management.types.${type}`)}</option>)}
          </SelectInput>
        </FormSection>
        <PermissionFields type={form.user_type} value={form.permissions} onChange={(permissions: Permission[]) => setForm({ ...form, permissions })} />
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={invite.isPending}>{t('management.sendInvitation')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
