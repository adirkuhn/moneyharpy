import { type FormEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useCreateAdministration } from '../administrations/queries'
import type { components } from '../api/schema'
import { Problems } from '../auth/AuthLayout'
import { formErrors, TextInput } from '../forms/Form'
import { currentLanguage } from '../i18n'
import { lookupAddress, useOnboardingOptions } from './queries'
import { SearchableSelect, type SelectOption } from './SearchableSelect'
import { Wizard } from './Wizard'
import { Loading } from '../layout/Loading'

type Options = components['schemas']['OnboardingOptions']
type Company = components['schemas']['CompanyMatch']

// Companies in these countries are identified through their national register.
const registerCountries = ['NL', 'BE']

export function ManualCompanyPage() {
  const options = useOnboardingOptions()
  return options.data ? <ManualForm options={options.data} /> : <Loading />
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

  const labels = {
    name: t('onboarding.companyName'), country: t('onboarding.country'), currency: t('onboarding.currency'), address1: t('onboarding.address'),
    address2: t('onboarding.additional'), zipcode: t('onboarding.zipcode'), city: t('onboarding.city'), chamber_of_commerce: t('onboarding.coc'),
  }
  const errors = formErrors(create.error, labels)
  const text = (key: Exclude<keyof typeof form, 'country' | 'currency'>, extra: Partial<React.ComponentProps<typeof TextInput>> = {}) => (
    <TextInput label={labels[key]} autoComplete="off" maxLength={255} value={form[key]} errors={errors.field(key)} onChange={set(key)} {...extra} />
  )
  const zipcodeField = text('zipcode', { maxLength: 32, width: 'small', onBlur: autofill })

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
      <Problems messages={errors.general} />
      {text('name', { required: true })}
      <SearchableSelect
        label={t('onboarding.country')}
        searchLabel={t('onboarding.findCountry')}
        emptyLabel={t('onboarding.noOption')}
        value={form.country}
        options={countryOptions}
        errors={errors.field('country')}
        onChange={(country) => {
          setForm({ ...form, country })
          setUnknownAddress(false)
        }}
      />
      {canLookUp && (
        <div className="field-pair">
          {zipcodeField}
          <TextInput label={t('onboarding.houseNumber')} width="small" autoComplete="off" maxLength={16} value={houseNumber} onChange={(e) => setHouseNumber(e.target.value)} onBlur={autofill} />
        </div>
      )}
      {unknownAddress && <p className="warning">{t('onboarding.unknownAddress')}</p>}
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
      {!canLookUp && zipcodeField}
      {text('city')}
      {showRegisterNumber && text('chamber_of_commerce', { maxLength: 64, width: 'medium' })}
      <SearchableSelect
        label={t('onboarding.currency')}
        searchLabel={t('onboarding.findCurrency')}
        emptyLabel={t('onboarding.noOption')}
        value={form.currency}
        options={currencyOptions}
        width="medium"
        errors={errors.field('currency')}
        onChange={(currency) => setForm({ ...form, currency })}
      />
    </Wizard>
  )
}
