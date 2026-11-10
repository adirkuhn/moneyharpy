import { type FormEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { type Administration, ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormNotice, FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { useOnboardingOptions } from '../onboarding/queries'
import { SearchableSelect } from '../onboarding/SearchableSelect'
import { countryName } from './format'
import { Loading } from '../layout/Loading'
import {
  type AdministrationChanges,
  type CustomField,
  type Identity,
  type IdentityFields,
  useCustomFields,
  useDeleteIdentity,
  useIdentity,
  useSaveIdentity,
} from './queries'

const textFields = [
  'company_name', 'legal_entity_type', 'address1', 'address2', 'zipcode', 'city', 'country', 'email', 'phone',
  'chamber_of_commerce', 'tax_number', 'bank_account_name', 'bank_account_number', 'bank_account_bic',
] as const
type TextField = (typeof textFields)[number]
type Reason = 'below_tax_threshold' | 'industry_exemption'

type Form = Record<TextField, string> & {
  noTaxNumber: boolean
  taxNumberAbsenceReason: Reason
  customFields: Record<string, string>
  profile: Required<Pick<AdministrationChanges, 'small_business_scheme' | 'company_activities_description' | 'company_website_url' | 'company_website_absent'>>
}

function toForm(identity: Identity | null, administration: Administration): Form {
  const text = Object.fromEntries(textFields.map((f) => [f, identity?.[f] ?? (f === 'country' ? 'NL' : '')])) as Record<TextField, string>
  return {
    ...text,
    noTaxNumber: !text.tax_number && identity?.tax_number_absence_reason != null,
    taxNumberAbsenceReason: identity?.tax_number_absence_reason ?? 'below_tax_threshold',
    customFields: Object.fromEntries((identity?.custom_fields ?? []).map((v) => [v.id, v.value])),
    profile: {
      small_business_scheme: administration.small_business_scheme,
      company_activities_description: administration.company_activities_description,
      company_website_url: administration.company_website_url,
      company_website_absent: administration.company_website_absent,
    },
  }
}

type Mode = 'default' | 'new' | 'edit'

/** Edits the company details, edits a sender address, or adds one. */
export function IdentityFormPage({ mode }: { mode: Mode }) {
  const { administrationId = '', identityId = '' } = useParams()
  const existing = useIdentity(administrationId, mode === 'edit' ? identityId : undefined)
  const administration = useAdministration(administrationId)
  const customFields = useCustomFields(administrationId)
  if (mode !== 'new' && existing.error instanceof ApiError && existing.error.status < 500) {
    return <NotFoundPage />
  }
  if ((mode !== 'new' && !existing.data) || !administration.data || !customFields.data) {
    return <Loading />
  }
  return (
    <IdentityForm
      key={existing.data?.id ?? 'new'}
      mode={mode}
      administration={administration.data}
      target={mode === 'edit' ? identityId : mode}
      initial={mode === 'new' ? null : existing.data ?? null}
      customFields={customFields.data.filter((f) => f.source === 'identity')}
    />
  )
}

type FormProps = {
  mode: Mode
  administration: Administration
  target: string
  initial: Identity | null
  customFields: CustomField[]
}

function IdentityForm({ mode, administration, target, initial, customFields }: FormProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const options = useOnboardingOptions()
  const [form, setForm] = useState<Form>(() => toForm(initial, administration))
  const [extraLine, setExtraLine] = useState(Boolean(initial?.address2))
  const [confirming, setConfirming] = useState(false)
  const save = useSaveIdentity(administration.id, target)
  const remove = useDeleteIdentity(administration.id)
  const companyDetails = mode === 'default'
  const back = `/${administration.id}/settings/company`
  const here = mode === 'default' ? `/${administration.id}/identities/edit_default` : mode === 'new' ? `/${administration.id}/identities/new` : `/${administration.id}/identities/${target}/edit`

  const countryOptions = useMemo(
    () =>
      (options.data?.countries ?? [form.country])
        .map((value) => ({ value, label: countryName(i18n.language, value) }))
        .sort((a, b) => a.label.localeCompare(b.label, i18n.language)),
    [options.data, form.country, i18n.language],
  )

  const labels = {
    company_name: t('onboarding.companyName'), legal_entity_type: t('settings.legalEntityType'), country: t('onboarding.country'),
    address1: t('onboarding.address'), address2: t('onboarding.additional'), zipcode: t('onboarding.zipcode'), city: t('onboarding.city'),
    email: t('settings.email'), phone: t('settings.phone'), chamber_of_commerce: t('settings.coc'), tax_number: t('settings.vat'),
    tax_number_absence_reason: t('settings.noVatReason'), bank_account_name: t('settings.bankHolder'),
    bank_account_number: t('settings.iban'), bank_account_bic: t('settings.bic'), custom_fields: t('settings.customFields'),
    small_business_scheme: t('settings.kor'), company_activities_description: t('settings.activities'),
    company_website_url: t('settings.website'), company_website_absent: t('settings.noWebsite'),
  }
  const errors = formErrors(save.error ?? remove.error, labels)

  const text = (key: TextField, extra: Partial<React.ComponentProps<typeof TextInput>> = {}) => (
    <TextInput
      label={labels[key]}
      autoComplete="off"
      maxLength={255}
      value={form[key]}
      errors={errors.field(key)}
      onChange={(e) => setForm({ ...form, [key]: e.target.value })}
      {...extra}
    />
  )
  const setProfile = (changes: Partial<Form['profile']>) => setForm({ ...form, profile: { ...form.profile, ...changes } })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const { noTaxNumber, taxNumberAbsenceReason, customFields: values, profile, ...identity } = form
    const fields: IdentityFields = {
      ...identity,
      tax_number_absence_reason: noTaxNumber && !identity.tax_number ? taxNumberAbsenceReason : '',
      ...(customFields.length > 0 ? { custom_fields_attributes: customFields.map((f) => ({ id: f.id, value: values[f.id] ?? '' })) } : {}),
    }
    save.mutate(
      { identity: fields, profile: companyDetails ? { ...profile, company_website_absent: profile.company_website_absent && !profile.company_website_url } : undefined },
      { onSuccess: () => navigate(back) },
    )
  }

  const title = mode === 'new' ? t('settings.newSender') : mode === 'default' ? t('settings.editDetails') : t('settings.editSender')
  const deleteAction = initial && !initial.default && (
    confirming ? (
      <button type="button" className="btn btn--danger" disabled={remove.isPending} onClick={() => remove.mutate(initial.id, { onSuccess: () => navigate(back) })}>
        {t('settings.confirmDelete')}
      </button>
    ) : (
      <button type="button" className="btn btn--secondary" onClick={() => setConfirming(true)}>{t('settings.delete')}</button>
    )
  )

  return (
    <Page title={title} back={{ to: back, label: t('settings.toCompany') }} actions={deleteAction || undefined}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        {companyDetails && <FormNotice>{t('settings.detailsNotice')}</FormNotice>}
        <Problems messages={errors.general} />

        <FormSection>
          {text('company_name', { required: true })}
          {text('legal_entity_type', { width: 'medium' })}
        </FormSection>

        <FormSection>
          {text('address1', {
            action: !extraLine && (
              <button type="button" className="icon-btn" aria-label={t('onboarding.addDetails')} onClick={() => setExtraLine(true)}>+</button>
            ),
          })}
          {extraLine &&
            text('address2', {
              action: (
                <button
                  type="button"
                  className="icon-btn"
                  aria-label={t('onboarding.removeDetails')}
                  onClick={() => {
                    setExtraLine(false)
                    setForm({ ...form, address2: '' })
                  }}
                >
                  &minus;
                </button>
              ),
            })}
          {text('zipcode', { width: 'small' })}
          {text('city')}
          <SearchableSelect
            label={labels.country}
            searchLabel={t('onboarding.findCountry')}
            emptyLabel={t('onboarding.noOption')}
            value={form.country}
            options={countryOptions}
            errors={errors.field('country')}
            onChange={(country) => setForm({ ...form, country })}
          />
        </FormSection>

        <FormSection>
          {text('email', { type: 'email', inputMode: 'email', autoCapitalize: 'off', spellCheck: false })}
          {text('phone', { type: 'tel', inputMode: 'tel', width: 'medium' })}
        </FormSection>

        <FormSection>
          {text('chamber_of_commerce', { width: 'medium' })}
          {text('tax_number', { width: 'medium' })}
          {!form.tax_number && (
            <CheckboxInput
              label={t('settings.noVat')}
              checked={form.noTaxNumber}
              onChange={(e) => setForm({ ...form, noTaxNumber: e.target.checked })}
            />
          )}
          {!form.tax_number && form.noTaxNumber && (
            <SelectInput
              label={labels.tax_number_absence_reason}
              value={form.taxNumberAbsenceReason}
              errors={errors.field('tax_number_absence_reason')}
              onChange={(e) => setForm({ ...form, taxNumberAbsenceReason: e.target.value as Reason })}
            >
              <option value="below_tax_threshold">{t('settings.reasons.below_tax_threshold')}</option>
              <option value="industry_exemption">{t('settings.reasons.industry_exemption')}</option>
            </SelectInput>
          )}
          {companyDetails && (
            <CheckboxInput
              label={labels.small_business_scheme}
              checked={form.profile.small_business_scheme}
              errors={errors.field('small_business_scheme')}
              onChange={(e) => setProfile({ small_business_scheme: e.target.checked })}
            />
          )}
        </FormSection>

        <FormSection>
          {text('bank_account_number', { autoCapitalize: 'characters', spellCheck: false })}
          {text('bank_account_bic', { width: 'medium', autoCapitalize: 'characters', spellCheck: false })}
          {text('bank_account_name')}
        </FormSection>

        {companyDetails && (
          <FormSection>
            <TextArea
              label={labels.company_activities_description}
              placeholder={t('settings.activitiesPlaceholder')}
              maxLength={1000}
              value={form.profile.company_activities_description}
              errors={errors.field('company_activities_description')}
              onChange={(e) => setProfile({ company_activities_description: e.target.value })}
            />
            <TextInput
              label={labels.company_website_url}
              type="url"
              inputMode="url"
              placeholder="https://"
              autoComplete="off"
              maxLength={255}
              value={form.profile.company_website_url}
              errors={errors.field('company_website_url')}
              onChange={(e) => setProfile({ company_website_url: e.target.value })}
            />
            {!form.profile.company_website_url && (
              <CheckboxInput
                label={labels.company_website_absent}
                checked={form.profile.company_website_absent}
                onChange={(e) => setProfile({ company_website_absent: e.target.checked })}
              />
            )}
          </FormSection>
        )}

        <FormSection title={t('settings.customFields')}>
          {errors.field('custom_fields') && <Problems messages={errors.field('custom_fields') ?? []} />}
          {customFields.map((f) => (
            <TextInput
              key={f.id}
              label={f.name}
              autoComplete="off"
              maxLength={255}
              value={form.customFields[f.id] ?? ''}
              onChange={(e) => setForm({ ...form, customFields: { ...form.customFields, [f.id]: e.target.value } })}
            />
          ))}
          <Link
            className="icon-link"
            to={`/${administration.id}/custom_fields/new?${new URLSearchParams({ source: 'identity', return_to: here })}`}
          >
            {t('settings.addCustomField')}
          </Link>
        </FormSection>

        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
