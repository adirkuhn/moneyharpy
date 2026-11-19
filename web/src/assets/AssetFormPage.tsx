import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { todayDay } from '../forms/calendar'
import { DateInput } from '../forms/DatePicker'
import { FormNotice, FormRow, formErrors, FormSection, SelectInput, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { planExample } from './plan'
import { type Asset, type AssetInput, useAsset, useSaveAsset } from './queries'
import './assets.css'

/** The lifespans Moneybird offers, in years; anything else is entered as a number. */
const lifespans = [5, 10, 30]

/** Adds an asset (/assets/new). */
export function NewAssetPage() {
  return <AssetForm />
}

/** Edits an asset (/assets/:assetId/edit); once it has value changes, only its name. */
export function EditAssetPage() {
  const { administrationId = '', assetId = '' } = useParams()
  const asset = useAsset(administrationId, assetId)
  if (asset.error instanceof ApiError && asset.error.status < 500) {
    return <NotFoundPage />
  }
  if (!asset.data) {
    return <Loading />
  }
  return <AssetForm key={String(asset.data.id)} asset={asset.data} />
}

const amountText = (amount: string | number | null | undefined) => (amount ? Number(amount).toFixed(2) : '')

/**
 * Moneybird's asset form: the name and category, then what it cost and when
 * it came into use, then how long it lasts and what it is worth at the end.
 */
function AssetForm({ asset }: { asset?: Asset }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const save = useSaveAsset(administrationId, asset?.id === undefined ? undefined : String(asset.id))
  const list = `/${administrationId}/assets`
  const locked = (asset?.value_changes ?? []).length > 0
  const years = asset?.value_change_plan?.lifespan_in_years

  const [name, setName] = useState(asset?.name ?? '')
  const [ledgerAccountId, setLedgerAccountId] = useState(asset?.ledger_account_id ?? '')
  const [value, setValue] = useState(amountText(asset?.purchase_value))
  const [date, setDate] = useState(asset?.purchase_date ?? todayDay())
  const [lifespan, setLifespan] = useState(asset && years !== undefined && !lifespans.includes(years) ? 'other' : String(years ?? 5))
  const [otherYears, setOtherYears] = useState(String(years ?? 1))
  const [residual, setResidual] = useState(amountText(asset?.value_change_plan?.residual_value))

  const currency = administration.data?.currency ?? 'EUR'
  const categories = (accounts.data ?? []).filter((a) => a.account_type === 'non_current_assets' && (a.active !== false || a.id === asset?.ledger_account_id))
  const labels: Record<string, string> = {
    name: t('assets.name'), ledger_account_id: t('assets.ledgerAccount'), purchase_value: t('assets.purchaseValue'),
    purchase_date: t('assets.commissioningDate'), lifespan_in_years: t('assets.lifespan'), residual_value: t('assets.residualValue'),
  }
  const errors = formErrors(save.error, labels)
  const chosenYears = Number(lifespan === 'other' ? otherYears : lifespan) || 0
  const example = planExample(date, value || '0', chosenYears, residual)
  const money = (c: number) => formatAmount(String(c / 100), currency, i18n.language)
  const month = (ym: string) => new Date(`${ym}-01T00:00:00`).toLocaleDateString(i18n.language, { month: 'long', year: 'numeric' })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: AssetInput = locked
      ? { name: name.trim() }
      : {
        name: name.trim(), ledger_account_id: ledgerAccountId, purchase_date: date, purchase_value: value.trim() || '0',
        value_change_plan_attributes: { lifespan_in_years: chosenYears, residual_value: residual.trim() || '0' },
      }
    save.mutate(body, { onSuccess: (saved) => navigate(`${list}/${saved.id}`) })
  }

  const moneyField = (label: string, field: string, amount: string, onChange: (v: string) => void, help?: string) => (
    <FormRow label={label} width="small" errors={errors.field(field)} help={help}>
      {({ labelId: _, ...control }) => (
        <span className="asset-money">
          <span className="asset-money__currency" aria-hidden="true">€</span>
          <input {...control} type="text" inputMode="decimal" placeholder="0.00" disabled={locked} value={amount} onChange={(e) => onChange(e.target.value)} />
        </span>
      )}
    </FormRow>
  )

  return (
    <Page wide title={asset ? t('assets.editTitle') : t('assets.newTitle')}>
      <form className="form-sheet asset-form" onSubmit={submit} noValidate aria-label={t('assets.form')}>
        {locked && <FormNotice>{t('assets.onlyName')}</FormNotice>}
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput
            label={labels.name ?? ''} width="medium" maxLength={255} placeholder={t('assets.namePlaceholder')} help={t('assets.nameHelp')}
            value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)}
          />
          <SelectInput
            label={labels.ledger_account_id ?? ''} width="medium" help={t('assets.ledgerAccountHelp')} disabled={locked}
            value={ledgerAccountId} errors={errors.field('ledger_account_id')} onChange={(e) => setLedgerAccountId(e.target.value)}
          >
            <option value="">{t('assets.selectOption')}</option>
            {categories.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </SelectInput>
        </FormSection>
        <FormSection>
          {moneyField(labels.purchase_value ?? '', 'purchase_value', value, setValue, t('assets.purchaseValueHelp'))}
          <DateInput label={labels.purchase_date ?? ''} value={date} disabled={locked} errors={errors.field('purchase_date')} onChange={setDate} />
        </FormSection>
        <FormSection>
          <FormRow label={labels.lifespan_in_years ?? ''} width="long" errors={errors.field('lifespan_in_years')}>
            {({ labelId: _, ...control }) => (
              <div className="asset-lifespan">
                <select {...control} disabled={locked} value={lifespan} onChange={(e) => setLifespan(e.target.value)}>
                  {lifespans.map((y) => <option key={y} value={String(y)}>{t('assets.years', { count: y })}</option>)}
                  <option value="other">{t('assets.otherLifespan')}</option>
                </select>
                {lifespan === 'other' && (
                  <>
                    <input
                      type="number" inputMode="decimal" min={1} max={1000} className="asset-lifespan__years" aria-label={t('assets.lifespanYears')}
                      disabled={locked} value={otherYears} onChange={(e) => setOtherYears(e.target.value)}
                    />
                    <span>{t('assets.yearsUnit')}</span>
                  </>
                )}
              </div>
            )}
          </FormRow>
          {moneyField(labels.residual_value ?? '', 'residual_value', residual, setResidual)}
        </FormSection>
        {example && (
          <section className="asset-example" aria-label={t('assets.example')}>
            <h4>{t('assets.example')}</h4>
            <p>
              {t('assets.exampleText', { start: month(example.start), total: money(example.total), years: chosenYears, monthly: money(example.monthly), end: month(example.end), residual: money(Math.round(Number(residual || 0) * 100)) })}
              {example.first !== undefined && example.last !== undefined && ` ${t('assets.exampleProRata', { first: money(example.first), last: money(example.last) })}`}
            </p>
          </section>
        )}
        <div className="form-sheet__actions form-sheet__actions--center">
          <button className="btn btn--primary" disabled={save.isPending}>{t('assets.save')}</button>
          <Link className="btn btn--secondary" to={asset ? `${list}/${asset.id}` : list}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
