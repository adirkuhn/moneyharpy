import { type FormEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useCreateAdministration } from '../administrations/queries'
import { errorMessages } from '../api/client'
import type { components } from '../api/schema'
import { Field, Problems } from '../auth/AuthLayout'
import { currentLanguage } from '../i18n'
import { lookupAddress, useOnboardingOptions } from './queries'
import { SearchableSelect, type SelectOption } from './SearchableSelect'
import { Wizard } from './Wizard'

type Options = components['schemas']['OnboardingOptions']
type Company = components['schemas']['CompanyMatch']

// Companies in these countries are identified through their national register.
const registerCountries = ['NL', 'BE']

export function ManualCompanyPage() {
  const options = useOnboardingOptions()
  return options.data ? <ManualForm options={options.data} /> : null
}

function display(language: string, type: 'region' | 'currency', code: string): string {
  try {
    return new Intl.DisplayNames([language], { type, fallback: 'code' }).of(code) ?? code
  } catch {
    return code
  }
}

function ManualForm({ options }: { options: Options }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const location = useLocation()
  const prefill = (location.state as { company?: Company } | null)?.company
  const requested = params.get('country') ?? ''

  const [form, setForm] = useState({
    name: prefill?.name ?? '',
    country: options.countries.includes(requested) ? requested : 'NL',
    currency: 'EUR',
    address1: prefill?.address1 ?? '',
    address2: '',
    zipcode: prefill?.zipcode ?? '',
    city: prefill?.city ?? '',
    chamber_of_commerce: prefill?.chamber_of_commerce ?? '',
  })
  const [houseNumber, setHouseNumber] = useState('')
  const [extraLine, setExtraLine] = useState(false)
  const [unknownAddress, setUnknownAddress] = useState(false)
  const create = useCreateAdministration()

  const countryOptions = useMemo<SelectOption[]>(
    () =>
      options.countries
        .map((value) => ({ value, label: display(i18n.language, 'region', value) }))
        .sort((a, b) => a.label.localeCompare(b.label, i18n.language)),
    [options.countries, i18n.language],
  )
  const currencyOptions = useMemo<SelectOption[]>(() => {
    const option = (value: string) => ({ value, label: `${value} - ${display(i18n.language, 'currency', value)}` })
    const popular = options.popular_currencies
    return [
      ...popular.map((value, index) => ({ ...option(value), separatorAfter: index === popular.length - 1 })),
      ...options.currencies.filter((c) => !popular.includes(c)).map(option),
    ]
  }, [options.currencies, options.popular_currencies, i18n.language])

  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [key]: e.target.value })
  const canLookUp = options.address_lookup_countries.includes(form.country)
  const showRegisterNumber = !registerCountries.includes(form.country)

  const autofill = async () => {
    if (!canLookUp || !form.zipcode.trim() || !houseNumber.trim()) {
      return
    }
    const found = await lookupAddress(form.zipcode, houseNumber).catch(() => null)
    setUnknownAddress(found === null)
    if (found) {
      setForm((f) => ({ ...f, address1: found.address1, zipcode: found.zipcode, city: found.city }))
    }
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    create.mutate(
      { ...form, language: currentLanguage(), time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone },
      { onSuccess: (a) => navigate(`${a.uri}/onboarding/first_action/new?origin=onboarding`) },
    )
  }

  const zipcodeField = <Field label={t('onboarding.zipcode')} autoComplete="off" maxLength={32} value={form.zipcode} onChange={set('zipcode')} onBlur={autofill} />
  const labels = {
    name: t('onboarding.companyName'), country: t('onboarding.country'), currency: t('onboarding.currency'), address1: t('onboarding.address'),
    address2: t('onboarding.additional'), zipcode: t('onboarding.zipcode'), city: t('onboarding.city'), chamber_of_commerce: t('onboarding.coc'),
  }

  return (
    <Wizard
      title={t('onboarding.createTitle')}
      subtitle={t('onboarding.createSubtitle')}
      onSubmit={submit}
      footer={
        <>
          <Link className="btn btn--secondary" to="/administrations">{t('common.cancel')}</Link>
          <button className="btn btn--primary" disabled={create.isPending}>{t('common.next')}</button>
        </>
      }
    >
      <Problems messages={errorMessages(create.error, labels)} />
      <Field label={t('onboarding.companyName')} autoComplete="off" required maxLength={255} value={form.name} onChange={set('name')} />
      <SearchableSelect
        label={t('onboarding.country')}
        searchLabel={t('onboarding.findCountry')}
        emptyLabel={t('onboarding.noOption')}
        value={form.country}
        options={countryOptions}
        onChange={(country) => {
          setForm({ ...form, country })
          setUnknownAddress(false)
        }}
      />
      {canLookUp && (
        <div className="field-pair">
          {zipcodeField}
          <Field label={t('onboarding.houseNumber')} autoComplete="off" maxLength={16} value={houseNumber} onChange={(e) => setHouseNumber(e.target.value)} onBlur={autofill} />
        </div>
      )}
      {unknownAddress && <p className="warning">{t('onboarding.unknownAddress')}</p>}
      <div className="field-with-action">
        <Field label={t('onboarding.address')} autoComplete="off" maxLength={255} value={form.address1} onChange={set('address1')} />
        {!extraLine && (
          <button type="button" className="icon-btn" aria-label={t('onboarding.addDetails')} onClick={() => setExtraLine(true)}>+</button>
        )}
      </div>
      {extraLine && (
        <div className="field-with-action">
          <Field label={t('onboarding.additional')} autoComplete="off" maxLength={255} value={form.address2} onChange={set('address2')} />
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
        </div>
      )}
      {!canLookUp && zipcodeField}
      <Field label={t('onboarding.city')} autoComplete="off" maxLength={255} value={form.city} onChange={set('city')} />
      {showRegisterNumber && <Field label={t('onboarding.coc')} autoComplete="off" maxLength={64} value={form.chamber_of_commerce} onChange={set('chamber_of_commerce')} />}
      <SearchableSelect
        label={t('onboarding.currency')}
        searchLabel={t('onboarding.findCurrency')}
        emptyLabel={t('onboarding.noOption')}
        value={form.currency}
        options={currencyOptions}
        onChange={(currency) => setForm({ ...form, currency })}
      />
    </Wizard>
  )
}
