import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import type { Language } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { Page } from '../layout/Page'
import { useOnboardingOptions } from '../onboarding/queries'
import { SearchableSelect } from '../onboarding/SearchableSelect'
import { useUpdateAdministration } from './queries'

function timeZones(current: string): string[] {
  const zones = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : []
  return zones.includes(current) ? zones : [current, ...zones]
}

export function ManagementSettingsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const options = useOnboardingOptions()
  const update = useUpdateAdministration(administrationId)
  const [form, setForm] = useState({ name: '', language: 'en' as Language, currency: 'EUR', time_zone: 'Europe/Amsterdam' })

  useEffect(() => {
    const a = administration.data
    if (a) {
      setForm({ name: a.name, language: a.language, currency: a.currency, time_zone: a.time_zone })
    }
  }, [administration.data])

  const currencyOptions = useMemo(() => {
    const names = new Intl.DisplayNames([i18n.language], { type: 'currency', fallback: 'code' })
    return (options.data?.currencies ?? [form.currency]).map((value) => ({ value, label: `${value} - ${names.of(value) ?? value}` }))
  }, [options.data, form.currency, i18n.language])
  const zoneOptions = useMemo(() => timeZones(form.time_zone).map((value) => ({ value, label: value })), [form.time_zone])

  if (!administration.data) {
    return null
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    update.mutate(form)
  }
  const labels = { name: t('register.name'), language: t('common.language'), currency: t('onboarding.currency'), time_zone: t('administrations.timeZone') }
  const errors = formErrors(update.error, labels)

  return (
    <Page title={t('settings.sections.administration')} subtitle={t('settings.managementSubtitle')}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <SelectInput label={labels.language} width="medium" value={form.language} errors={errors.field('language')} onChange={(e) => setForm({ ...form, language: e.target.value as Language })}>
            <option value="en">English</option>
            <option value="nl">Nederlands</option>
            <option value="nl-be">Nederlands (België)</option>
          </SelectInput>
        </FormSection>
        <FormSection>
          <SearchableSelect
            label={labels.currency}
            searchLabel={t('onboarding.findCurrency')}
            emptyLabel={t('onboarding.noOption')}
            value={form.currency}
            options={currencyOptions}
            width="medium"
            errors={errors.field('currency')}
            onChange={(currency) => setForm({ ...form, currency })}
          />
          <SearchableSelect
            label={labels.time_zone}
            searchLabel={t('settings.findTimeZone')}
            emptyLabel={t('onboarding.noOption')}
            value={form.time_zone}
            options={zoneOptions}
            width="medium"
            errors={errors.field('time_zone')}
            onChange={(time_zone) => setForm({ ...form, time_zone })}
          />
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={update.isPending}>{t('settings.save')}</button>
          {update.isSuccess && <p role="status" className="saved">{t('settings.saved')}</p>}
        </div>
      </form>
    </Page>
  )
}
