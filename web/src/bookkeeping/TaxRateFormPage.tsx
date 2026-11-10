import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type BookkeepingSettings, type TaxRate, type TaxRateType, taxRateTypes, useBookkeepingSettings, useSaveTaxRate, useTaxRate } from './queries'

/** Adds a VAT rate (/tax_rates/new) or edits one (/tax_rates/:id). */
export function TaxRateFormPage() {
  const { administrationId = '', taxRateId } = useParams()
  const settings = useBookkeepingSettings(administrationId)
  const existing = useTaxRate(administrationId, taxRateId)
  if (existing.error instanceof ApiError && existing.error.status < 500) {
    return <NotFoundPage />
  }
  if (!settings.data || (taxRateId && !existing.data)) {
    return <Loading />
  }
  return <TaxRateForm key={existing.data?.id ?? 'new'} administrationId={administrationId} settings={settings.data} rate={existing.data ?? null} />
}

type Props = { administrationId: string; settings: BookkeepingSettings; rate: TaxRate | null }

function TaxRateForm({ administrationId, settings, rate }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveTaxRate(administrationId, rate?.id)
  const [form, setForm] = useState({
    name: rate?.name ?? '',
    percentage: rate ? String(rate.percentage ?? '') : '',
    tax_rate_type: (rate?.tax_rate_type ?? 'sales_invoice') as TaxRateType,
    show_tax: rate?.show_tax ?? true,
    active: rate?.active ?? true,
    country: rate?.country ?? '',
    tax_report_boxes: rate?.tax_report_boxes ?? [],
  })
  const back = `/${administrationId}/settings/bookkeeping#vat-rates`
  const labels = {
    name: t('bookkeeping.rateName'), percentage: t('bookkeeping.percentage'), tax_rate_type: t('bookkeeping.rateType'),
    country: t('bookkeeping.rateCountry'), tax_report_boxes: t('bookkeeping.reportColumn'),
  }
  const errors = formErrors(save.error, labels)
  const toggleBox = (code: string, on: boolean) =>
    setForm({ ...form, tax_report_boxes: on ? [...form.tax_report_boxes, code] : form.tax_report_boxes.filter((b) => b !== code) })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(form, { onSuccess: () => navigate(back) })
  }

  return (
    <Page title={rate ? t('bookkeeping.editRate') : t('bookkeeping.newRate')} back={{ to: back, label: t('bookkeeping.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <TextInput label={labels.percentage} required inputMode="decimal" width="small" maxLength={6} help="%" value={form.percentage} errors={errors.field('percentage')} onChange={(e) => setForm({ ...form, percentage: e.target.value })} />
          <SelectInput label={labels.tax_rate_type} width="medium" value={form.tax_rate_type} errors={errors.field('tax_rate_type')} onChange={(e) => setForm({ ...form, tax_rate_type: e.target.value as TaxRateType })}>
            {taxRateTypes.map((type) => <option key={type} value={type}>{t(`bookkeeping.rateTypes.${type}`)}</option>)}
          </SelectInput>
          <TextInput label={labels.country} width="small" maxLength={2} autoComplete="off" help={t('bookkeeping.rateCountryHelp')} value={form.country} errors={errors.field('country')} onChange={(e) => setForm({ ...form, country: e.target.value.toUpperCase() })} />
          <CheckboxInput label={t('bookkeeping.showTax')} help={t('bookkeeping.showTaxHelp')} checked={form.show_tax} onChange={(e) => setForm({ ...form, show_tax: e.target.checked })} />
          <CheckboxInput label={t('bookkeeping.active')} checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} />
        </FormSection>
        {settings.tax_report_boxes.length > 0 && (
          <FormSection title={labels.tax_report_boxes}>
            {errors.field('tax_report_boxes') && <Problems messages={errors.field('tax_report_boxes') ?? []} />}
            {settings.tax_report_boxes.map((b) => (
              <CheckboxInput key={b.code} label={`${b.code.replace(/^[A-Z]{2}\//, '')} ${b.name}`} checked={form.tax_report_boxes.includes(b.code)} onChange={(e) => toggleBox(b.code, e.target.checked)} />
            ))}
          </FormSection>
        )}
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
