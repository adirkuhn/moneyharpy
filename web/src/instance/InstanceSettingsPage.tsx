import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, errorMessages, type InstanceSettings, unwrap, type UpdateInstanceSettings } from '../api/client'
import { Field, Problems } from '../auth/AuthLayout'
import { useCurrentUser } from '../auth/queries'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'

const defaultSmtpPort = 587

export function InstanceSettingsPage() {
  const me = useCurrentUser()
  if (!me.data?.instance_admin) {
    return <NotFoundPage />
  }
  return <SettingsForm />
}

function SettingsForm() {
  const { t } = useTranslation()
  const labels = {
    google_client_id: t('instance.googleId'),
    google_client_secret: t('instance.googleSecret'),
    lm2_url: t('instance.lm2Url'),
    lm2_app: t('instance.lm2App'),
    kvk_api_key: t('instance.kvkKey'),
    smtp_host: t('instance.smtpHost'),
    smtp_port: t('instance.smtpPort'),
    smtp_from: t('instance.smtpFrom'),
    smtp_security: t('instance.smtpSecurity'),
  }
  const client = useQueryClient()
  const settings = useQuery({ queryKey: ['instance-settings'], queryFn: () => unwrap(api.GET('/api/instance/settings')) })
  const [form, setForm] = useState<InstanceSettings | null>(null)
  const [secret, setSecret] = useState('')
  const [kvkKey, setKvkKey] = useState('')
  const [smtpPassword, setSmtpPassword] = useState('')
  const me = useCurrentUser()

  useEffect(() => {
    if (settings.data) {
      setForm({ ...settings.data, smtp_port: settings.data.smtp_port || defaultSmtpPort })
    }
  }, [settings.data])

  const save = useMutation({
    mutationFn: (body: UpdateInstanceSettings) => unwrap(api.PATCH('/api/instance/settings', { body })),
    onSuccess: (saved) => {
      setSecret('')
      setKvkKey('')
      setSmtpPassword('')
      client.setQueryData(['instance-settings'], saved)
      void client.invalidateQueries({ queryKey: ['providers'] })
    },
  })

  const testEmail = useMutation({ mutationFn: () => unwrap(api.POST('/api/instance/settings/test_email')) })

  if (!form) {
    return null
  }

  const security = (form.smtp_security || 'starttls') as NonNullable<UpdateInstanceSettings['smtp_security']>

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate({
      registration_open: form.registration_open,
      google_enabled: form.google_enabled,
      google_client_id: form.google_client_id,
      lm2_enabled: form.lm2_enabled,
      lm2_url: form.lm2_url,
      lm2_app: form.lm2_app,
      kvk_enabled: form.kvk_enabled,
      address_lookup_enabled: form.address_lookup_enabled,
      ...(kvkKey ? { kvk_api_key: kvkKey } : {}),
      smtp_enabled: form.smtp_enabled,
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_from: form.smtp_from,
      smtp_security: security,
      ...(smtpPassword ? { smtp_password: smtpPassword } : {}),
      ...(secret ? { google_client_secret: secret } : {}),
    })
  }

  return (
    <Page title={t('instance.title')}>
      <form className="form" onSubmit={submit}>
        <Problems messages={errorMessages(save.error, labels)} />
        {save.isSuccess && <p role="status" className="saved">{t('instance.saved')}</p>}
        <label className="check">
          <input type="checkbox" checked={form.registration_open} onChange={(e) => setForm({ ...form, registration_open: e.target.checked })} />
          <span>{t('instance.register')}</span>
        </label>
        <h2>{t('instance.googleTitle')}</h2>
        <label className="check">
          <input type="checkbox" checked={form.google_enabled} onChange={(e) => setForm({ ...form, google_enabled: e.target.checked })} />
          <span>{t('instance.googleEnable')}</span>
        </label>
        <Field label={t('instance.googleId')} value={form.google_client_id} onChange={(e) => setForm({ ...form, google_client_id: e.target.value })} />
        <Field
          label={t('instance.googleSecret')}
          type="password"
          autoComplete="off"
          placeholder={form.google_client_secret_set ? t('instance.stored') : ''}
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
        />
        <h2>{t('instance.lm2Title')}</h2>
        <label className="check">
          <input type="checkbox" checked={form.lm2_enabled} onChange={(e) => setForm({ ...form, lm2_enabled: e.target.checked })} />
          <span>{t('instance.lm2Enable')}</span>
        </label>
        <Field label={t('instance.lm2Url')} type="url" placeholder="https://lm.example.com" value={form.lm2_url} onChange={(e) => setForm({ ...form, lm2_url: e.target.value })} />
        <Field label={t('instance.lm2App')} value={form.lm2_app} onChange={(e) => setForm({ ...form, lm2_app: e.target.value })} />
        <h2>{t('instance.kvkTitle')}</h2>
        <label className="check">
          <input type="checkbox" checked={form.kvk_enabled} onChange={(e) => setForm({ ...form, kvk_enabled: e.target.checked })} />
          <span>{t('instance.kvkEnable')}</span>
        </label>
        <Field
          label={t('instance.kvkKey')}
          type="password"
          autoComplete="off"
          placeholder={form.kvk_api_key_set ? t('instance.stored') : ''}
          value={kvkKey}
          onChange={(e) => setKvkKey(e.target.value)}
        />
        <h2>{t('instance.addressTitle')}</h2>
        <label className="check">
          <input type="checkbox" checked={form.address_lookup_enabled} onChange={(e) => setForm({ ...form, address_lookup_enabled: e.target.checked })} />
          <span>{t('instance.addressEnable')}</span>
        </label>
        <p className="hint">{t('instance.addressHint')}</p>
        <h2>{t('instance.smtpTitle')}</h2>
        <label className="check">
          <input type="checkbox" checked={form.smtp_enabled} onChange={(e) => setForm({ ...form, smtp_enabled: e.target.checked })} />
          <span>{t('instance.smtpEnable')}</span>
        </label>
        <p className="hint">{t('instance.smtpHint')}</p>
        <Field label={t('instance.smtpHost')} autoComplete="off" value={form.smtp_host} onChange={(e) => setForm({ ...form, smtp_host: e.target.value })} />
        <Field label={t('instance.smtpPort')} type="number" min={1} max={65535} value={form.smtp_port || ''} onChange={(e) => setForm({ ...form, smtp_port: Number(e.target.value) })} />
        <label className="field">
          <span>{t('instance.smtpSecurity')}</span>
          <select value={security} onChange={(e) => setForm({ ...form, smtp_security: e.target.value })}>
            <option value="starttls">{t('instance.smtpStarttls')}</option>
            <option value="tls">{t('instance.smtpTls')}</option>
            <option value="none">{t('instance.smtpNone')}</option>
          </select>
        </label>
        <Field label={t('instance.smtpUser')} autoComplete="off" value={form.smtp_username} onChange={(e) => setForm({ ...form, smtp_username: e.target.value })} />
        <Field
          label={t('instance.smtpPassword')}
          type="password"
          autoComplete="off"
          placeholder={form.smtp_password_set ? t('instance.stored') : ''}
          value={smtpPassword}
          onChange={(e) => setSmtpPassword(e.target.value)}
        />
        <Field label={t('instance.smtpFrom')} type="email" autoComplete="off" value={form.smtp_from} onChange={(e) => setForm({ ...form, smtp_from: e.target.value })} />
        <div className="form__actions">
          <button type="button" className="btn btn--secondary" disabled={!settings.data?.smtp_enabled || testEmail.isPending} onClick={() => testEmail.mutate()}>
            {t('instance.testEmail')}
          </button>
          {testEmail.isSuccess && <span className="saved">{t('instance.testSent', { email: me.data?.email })}</span>}
        </div>
        <Problems messages={errorMessages(testEmail.error, {})} />
        <button className="btn btn--primary" disabled={save.isPending}>{t('instance.save')}</button>
      </form>
    </Page>
  )
}
