import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { errorMessages } from '../api/client'
import { useAcceptInvitation, useInvitationPreview } from '../management/queries'
import { AuthLayout, Problems } from './AuthLayout'
import { useCurrentUser } from './queries'

/** Opens an invitation link: accept when signed in, otherwise sign in or create an account first. */
export function InvitationPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { token = '' } = useParams()
  const me = useCurrentUser()
  const preview = useInvitationPreview(token)
  const accept = useAcceptInvitation(token)
  const here = `/invitations/${token}`

  if (preview.isError) {
    return (
      <AuthLayout title={t('invitation.notFoundTitle')}>
        <p>{t('invitation.notFound')}</p>
        <div className="auth__actions"><Link className="btn btn--secondary" to="/administrations">{t('notFound.back')}</Link></div>
      </AuthLayout>
    )
  }
  if (!preview.data || me.isLoading) {
    return <AuthLayout title={t('common.loading')}><p className="hint">{t('common.loading')}</p></AuthLayout>
  }
  const v = preview.data
  return (
    <AuthLayout title={t('invitation.title', { name: v.administration_name })}>
      <p>{t('invitation.body', { inviter: v.inviter_name || t('invitation.someone'), name: v.administration_name, type: t(`management.types.${v.user_type}`) })}</p>
      <p className="hint">{t('invitation.sentTo', { email: v.email })}</p>
      <Problems messages={v.open ? errorMessages(accept.error, {}) : [t('invitation.closed')]} />
      {v.open && (me.data ? (
        <button type="button" className="btn btn--primary btn--wide" disabled={accept.isPending} onClick={() => accept.mutate(undefined, { onSuccess: (a) => navigate(a.uri, { replace: true }) })}>
          {t('invitation.accept')}
        </button>
      ) : (
        <div className="auth__actions">
          <Link className="btn btn--primary" to="/login" state={{ from: here }}>{t('invitation.signIn')}</Link>
          <Link className="btn btn--secondary" to={`/register?invitation=${encodeURIComponent(token)}&email=${encodeURIComponent(v.email)}`}>{t('invitation.register')}</Link>
        </div>
      ))}
    </AuthLayout>
  )
}
