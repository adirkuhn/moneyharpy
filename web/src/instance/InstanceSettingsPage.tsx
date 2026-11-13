import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api, ApiError, errorMessages, type InstanceSettings, unwrap, type UpdateInstanceSettings } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { useCurrentUser } from '../auth/queries'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { Block } from '../settings/Block'
import { Loading } from '../layout/Loading'
import { useSupportGrants } from '../management/queries'

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
    smtp_username: t('instance.smtpUser'),
    smtp_password: t('instance.smtpPassword'),
    storage_endpoint: t('instance.storageEndpoint'),
    storage_region: t('instance.storageRegion'),
    storage_bucket: t('instance.storageBucket'),
    storage_access_key: t('instance.storageAccessKey'),
    storage_secret_key: t('instance.storageSecretKey'),
    recognition_ai_provider: t('instance.aiProvider'),
    recognition_ai_url: t('instance.aiUrl'),
    recognition_ai_model: t('instance.aiModel'),
    recognition_ai_api_key: t('instance.aiKey'),
  }
  const client = useQueryClient()
  const settings = useQuery({ queryKey: ['instance-settings'], queryFn: () => unwrap(api.GET('/api/instance/settings')) })
  const [form, setForm] = useState<InstanceSettings | null>(null)
  const [secret, setSecret] = useState('')
  const [kvkKey, setKvkKey] = useState('')
  const [smtpPassword, setSmtpPassword] = useState('')
  const [storageSecret, setStorageSecret] = useState('')
  const [aiKey, setAiKey] = useState('')
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
      setStorageSecret('')
      setAiKey('')
      client.setQueryData(['instance-settings'], saved)
      void client.invalidateQueries({ queryKey: ['providers'] })
    },
  })

  const testEmail = useMutation({ mutationFn: () => unwrap(api.POST('/api/instance/settings/test_email')) })
  const testStorage = useMutation({ mutationFn: () => unwrap(api.POST('/api/instance/settings/test_storage')) })
  const testKvkWebsite = useMutation({ mutationFn: () => unwrap(api.POST('/api/instance/settings/test_kvk_website_search')) })
  const testRecognition = useMutation({ mutationFn: () => unwrap(api.POST('/api/instance/settings/test_recognition')) })

  if (!form) {
    return <Loading />
  }

  const security = (form.smtp_security || 'starttls') as NonNullable<UpdateInstanceSettings['smtp_security']>
  const provider = form.recognition_ai_provider || 'ollama'

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
      kvk_website_search_enabled: form.kvk_website_search_enabled,
      address_lookup_enabled: form.address_lookup_enabled,
      webhooks_allow_local: form.webhooks_allow_local,
      ...(kvkKey ? { kvk_api_key: kvkKey } : {}),
      smtp_enabled: form.smtp_enabled,
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_username: form.smtp_username,
      smtp_from: form.smtp_from,
      smtp_security: security,
      ...(smtpPassword ? { smtp_password: smtpPassword } : {}),
      storage_enabled: form.storage_enabled,
      storage_endpoint: form.storage_endpoint,
      storage_region: form.storage_region,
      storage_bucket: form.storage_bucket,
      storage_access_key: form.storage_access_key,
      storage_path_style: form.storage_path_style,
      ...(storageSecret ? { storage_secret_key: storageSecret } : {}),
      ...(secret ? { google_client_secret: secret } : {}),
      recognition_ai_enabled: form.recognition_ai_enabled,
      recognition_ai_provider: provider,
      recognition_ai_url: form.recognition_ai_url,
      recognition_ai_model: form.recognition_ai_model,
      ...(aiKey ? { recognition_ai_api_key: aiKey } : {}),
    })
  }

  const errors = formErrors(save.error, labels)
  const check = (key: 'registration_open' | 'google_enabled' | 'lm2_enabled' | 'kvk_enabled' | 'kvk_website_search_enabled' | 'address_lookup_enabled' | 'webhooks_allow_local' | 'smtp_enabled' | 'storage_enabled' | 'storage_path_style' | 'recognition_ai_enabled', label: string, help?: string) => (
    <CheckboxInput label={label} help={help} checked={form[key]} onChange={(e) => setForm({ ...form, [key]: e.target.checked })} />
  )
  const text = (
    key: 'google_client_id' | 'lm2_url' | 'lm2_app' | 'smtp_host' | 'smtp_username' | 'smtp_from' | 'storage_endpoint' | 'storage_region' | 'storage_bucket' | 'storage_access_key' | 'recognition_ai_url' | 'recognition_ai_model',
    label: string,
    extra: Partial<React.ComponentProps<typeof TextInput>> = {},
  ) => (
    <TextInput label={label} autoComplete="off" value={form[key]} errors={errors.field(key)} onChange={(e) => setForm({ ...form, [key]: e.target.value })} {...extra} />
  )

  return (
    <Page title={t('instance.title')} subtitle={t('instance.subtitle')}>
      <Block id="services" title={t('instance.servicesTitle')} subtitle={t('instance.servicesSubtitle')}>
      <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate aria-label={t('instance.servicesTitle')}>
        <Problems messages={errors.general} />
        <FormSection>{check('registration_open', t('instance.register'))}</FormSection>
        <FormSection title={t('instance.googleTitle')}>
          {check('google_enabled', t('instance.googleEnable'))}
          {text('google_client_id', labels.google_client_id)}
          <TextInput
            label={labels.google_client_secret}
            type="password"
            autoComplete="off"
            placeholder={form.google_client_secret_set ? t('instance.stored') : ''}
            value={secret}
            errors={errors.field('google_client_secret')}
            onChange={(e) => setSecret(e.target.value)}
          />
        </FormSection>
        <FormSection title={t('instance.lm2Title')}>
          {check('lm2_enabled', t('instance.lm2Enable'))}
          {text('lm2_url', labels.lm2_url, { type: 'url', placeholder: 'https://lm.example.com' })}
          {text('lm2_app', labels.lm2_app, { width: 'medium' })}
        </FormSection>
        <FormSection title={t('instance.kvkTitle')}>
          {check('kvk_enabled', t('instance.kvkEnable'))}
          <TextInput
            label={labels.kvk_api_key}
            type="password"
            autoComplete="off"
            placeholder={form.kvk_api_key_set ? t('instance.stored') : ''}
            value={kvkKey}
            errors={errors.field('kvk_api_key')}
            onChange={(e) => setKvkKey(e.target.value)}
          />
          {check('kvk_website_search_enabled', t('instance.kvkWebsiteEnable'), t('instance.kvkWebsiteHint'))}
          <div className="form-input form-input--check">
            <span className="form-input__label" aria-hidden="true" />
            <div className="form-input__body form__actions">
              <button type="button" className="btn btn--secondary" disabled={testKvkWebsite.isPending} onClick={() => testKvkWebsite.mutate()}>
                {t('instance.testKvkWebsite')}
              </button>
              {testKvkWebsite.isSuccess && <span role="status" className="saved">{t('instance.kvkWebsiteWorks')}</span>}
            </div>
          </div>
          {testKvkWebsite.isError && <Problems messages={[kvkWebsiteProblem(testKvkWebsite.error, t)]} />}
        </FormSection>
        <FormSection title={t('instance.addressTitle')}>
          {check('address_lookup_enabled', t('instance.addressEnable'), t('instance.addressHint'))}
          {check('webhooks_allow_local', t('instance.webhooksLocal'), t('instance.webhooksLocalHint'))}
        </FormSection>
        <FormSection title={t('instance.smtpTitle')}>
          {check('smtp_enabled', t('instance.smtpEnable'), t('instance.smtpHint'))}
          {text('smtp_host', labels.smtp_host)}
          <TextInput
            label={labels.smtp_port}
            type="number"
            width="small"
            min={1}
            max={65535}
            value={form.smtp_port || ''}
            errors={errors.field('smtp_port')}
            onChange={(e) => setForm({ ...form, smtp_port: Number(e.target.value) })}
          />
          <SelectInput label={labels.smtp_security} width="medium" value={security} errors={errors.field('smtp_security')} onChange={(e) => setForm({ ...form, smtp_security: e.target.value })}>
            <option value="starttls">{t('instance.smtpStarttls')}</option>
            <option value="tls">{t('instance.smtpTls')}</option>
            <option value="none">{t('instance.smtpNone')}</option>
          </SelectInput>
          {text('smtp_username', t('instance.smtpUser'))}
          <TextInput
            label={t('instance.smtpPassword')}
            type="password"
            autoComplete="off"
            placeholder={form.smtp_password_set ? t('instance.stored') : ''}
            value={smtpPassword}
            errors={errors.field('smtp_password')}
            onChange={(e) => setSmtpPassword(e.target.value)}
          />
          {text('smtp_from', labels.smtp_from, { type: 'email' })}
          <div className="form-input form-input--check">
            <span className="form-input__label" aria-hidden="true" />
            <div className="form-input__body form__actions">
              <button type="button" className="btn btn--secondary" disabled={!settings.data?.smtp_enabled || testEmail.isPending} onClick={() => testEmail.mutate()}>
                {t('instance.testEmail')}
              </button>
              {testEmail.isSuccess && <span className="saved">{t('instance.testSent', { email: me.data?.email })}</span>}
            </div>
          </div>
          <Problems messages={errorMessages(testEmail.error, {})} />
        </FormSection>
        <FormSection title={t('instance.storageTitle')}>
          {check('storage_enabled', t('instance.storageEnable'), t('instance.storageHint'))}
          {text('storage_endpoint', labels.storage_endpoint, { type: 'url', placeholder: 'https://s3.eu-west-1.amazonaws.com' })}
          {text('storage_region', labels.storage_region, { width: 'medium', placeholder: 'us-east-1' })}
          {text('storage_bucket', labels.storage_bucket, { width: 'medium' })}
          {text('storage_access_key', labels.storage_access_key)}
          <TextInput
            label={labels.storage_secret_key}
            type="password"
            autoComplete="off"
            placeholder={form.storage_secret_key_set ? t('instance.stored') : ''}
            value={storageSecret}
            errors={errors.field('storage_secret_key')}
            onChange={(e) => setStorageSecret(e.target.value)}
          />
          {check('storage_path_style', t('instance.storagePathStyle'))}
          <div className="form-input form-input--check">
            <span className="form-input__label" aria-hidden="true" />
            <div className="form-input__body form__actions">
              <button type="button" className="btn btn--secondary" disabled={!settings.data?.storage_enabled || testStorage.isPending} onClick={() => testStorage.mutate()}>
                {t('instance.testStorage')}
              </button>
              {testStorage.isSuccess && <span className="saved">{t('instance.storageWorks')}</span>}
            </div>
          </div>
          <Problems messages={errorMessages(testStorage.error, {})} />
        </FormSection>
        <FormSection title={t('instance.recognitionTitle')}>
          <p className="hint">{t('instance.recognitionHint')}</p>
          {check('recognition_ai_enabled', t('instance.aiEnable'), t('instance.aiWarning'))}
          <SelectInput
            label={labels.recognition_ai_provider}
            width="medium"
            value={provider}
            errors={errors.field('recognition_ai_provider')}
            onChange={(e) => setForm({ ...form, recognition_ai_provider: e.target.value as typeof provider })}
          >
            <option value="ollama">{t('instance.aiOllama')}</option>
            <option value="anthropic">{t('instance.aiAnthropic')}</option>
          </SelectInput>
          {provider === 'ollama' && text('recognition_ai_url', labels.recognition_ai_url, { type: 'url', placeholder: 'http://ollama:11434' })}
          {text('recognition_ai_model', labels.recognition_ai_model, { width: 'medium', placeholder: provider === 'anthropic' ? 'claude-opus-5-5' : 'qwen2.5vl' })}
          {provider === 'anthropic' && (
            <TextInput
              label={labels.recognition_ai_api_key}
              type="password"
              autoComplete="off"
              placeholder={form.recognition_ai_api_key_set ? t('instance.stored') : ''}
              value={aiKey}
              errors={errors.field('recognition_ai_api_key')}
              onChange={(e) => setAiKey(e.target.value)}
            />
          )}
          <div className="form-input form-input--check">
            <span className="form-input__label" aria-hidden="true" />
            <div className="form-input__body form__actions">
              <button type="button" className="btn btn--secondary" disabled={testRecognition.isPending} onClick={() => testRecognition.mutate()}>
                {t('instance.checkRecognition')}
              </button>
              {testRecognition.data && (
                <ul role="status" className="recognition-check">
                  <li className={testRecognition.data.ocr_available ? 'saved' : 'recognition-check__problem'}>
                    {t(testRecognition.data.ocr_available ? 'instance.ocrAvailable' : 'instance.ocrUnavailable')}
                  </li>
                  <li className={testRecognition.data.ai === 'unavailable' ? 'recognition-check__problem' : 'saved'}>
                    {t({ off: 'instance.aiOff', ok: 'instance.aiOk', unavailable: 'instance.aiUnavailable' }[testRecognition.data.ai])}
                    {testRecognition.data.ai === 'unavailable' && testRecognition.data.message ? `: ${testRecognition.data.message}` : ''}
                  </li>
                  {testRecognition.data.ai !== 'unavailable' && !testRecognition.data.ocr_available && testRecognition.data.message && (
                    <li className="recognition-check__problem">{testRecognition.data.message}</li>
                  )}
                </ul>
              )}
            </div>
          </div>
          <Problems messages={errorMessages(testRecognition.error, {})} />
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('instance.save')}</button>
          {save.isSuccess && <p role="status" className="saved">{t('instance.saved')}</p>}
        </div>
      </form>
      </Block>
      <SupportGrants />
    </Page>
  )
}

/** Why the KVK website search check failed: KVK not answering, blocking the request, or too many searches. */
function kvkWebsiteProblem(error: unknown, t: (key: string) => string) {
  switch (error instanceof ApiError ? error.code : '') {
    case 'kvk_unavailable':
      return t('instance.kvkWebsiteUnavailable')
    case 'kvk_blocked':
      return t('instance.kvkWebsiteBlocked')
    case 'kvk_rate_limited':
      return t('instance.kvkWebsiteBusy')
    default:
      return errorMessages(error, {})[0] ?? t('common.genericError')
  }
}

/** The administrations that let the instance admins in right now. */
function SupportGrants() {
  const { t, i18n } = useTranslation()
  const grants = useSupportGrants()
  return (
    <section className="settings-block" aria-labelledby="support-grants">
      <header className="settings-block__header">
        <div>
          <h2 id="support-grants">{t('management.grantsTitle')}</h2>
          <p className="hint">{t('management.grantsSubtitle')}</p>
        </div>
      </header>
      {grants.data?.length ? (
        <ul className="ledger">
          {grants.data.map((g) => (
            <li className="ledger__row" key={g.administration_id}>
              <Link className="ledger__name" to={g.uri}>{g.name}</Link>
              <span className="hint">{t('management.activeUntil', { date: new Date(g.until).toLocaleString(i18n.language, { dateStyle: 'medium', timeStyle: 'short' }) })}</span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="empty">{t('management.noGrants')}</p>
      )}
    </section>
  )
}
