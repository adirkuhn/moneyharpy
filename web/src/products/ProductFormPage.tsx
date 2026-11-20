import { ActionLink } from '../layout/ActionLink'
import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { bookable } from '../banking/queries'
import { categoryOption, type LedgerAccount, type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { currencyOptions } from '../forms/currencies'
import { CheckboxInput, FormRow, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type Product, type ProductChanges, productFrequencyTypes, productImageURL, useProduct, useProductImage, useSaveProduct } from './queries'
import './products.css'

/** Moneybird's choices of how often a product is billed, as frequency:type; "other" opens the number and unit. */
const presets = ['', '1:month', '1:quarter', '6:month', '1:year'] as const
const presetName: Record<(typeof presets)[number], string> = { '': 'none', '1:month': 'month', '1:quarter': 'quarter', '6:month': 'halfYear', '1:year': 'year' }
const productTypes = ['digital_service', 'service', 'product'] as const
const vatRateTypes = ['standard', 'reduced'] as const

/** A new product (/products/new) or one being changed (/products/:productId/edit), in Moneybird's form. */
export function ProductFormPage() {
  const { administrationId = '', productId } = useParams()
  const product = useProduct(administrationId, productId)
  const rates = useTaxRates(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  if (productId && product.error instanceof ApiError && product.error.status < 500) {
    return <NotFoundPage />
  }
  if ((productId && !product.data) || !rates.data || !accounts.data) {
    return <Loading />
  }
  const current = product.data ?? null
  const usable = rates.data.filter((r) => r.tax_rate_type === 'sales_invoice' && (r.active || r.id === current?.tax_rate_id))
  return <ProductForm key={current?.id ?? 'new'} administrationId={administrationId} product={current} rates={usable} accounts={accounts.data} />
}

/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))

/** The preset of a product's period, "other" when none fits. */
function presetOf(p: Product | null): string {
  if (!p?.frequency_type) {
    return ''
  }
  const key = `${p.frequency ?? 1}:${p.frequency_type}`
  return (presets as readonly string[]).includes(key) ? key : 'other'
}

type FormProps = { administrationId: string; product: Product | null; rates: TaxRate[]; accounts: LedgerAccount[] }

function ProductForm({ administrationId, product, rates, accounts }: FormProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const ids = useId()
  const save = useSaveProduct(administrationId, product?.id)
  const image = useProductImage(administrationId)
  const administration = useAdministration(administrationId)
  // Moneybird's first choice is its uncategorized income, where a product is booked until it gets a category.
  const uncategorized = accounts.find((a) => a.system_role === 'uncategorized_income')
  const choices = bookable(accounts, 'sales_invoice').flatMap((g) => g.rows)
  const categories = uncategorized && !choices.some((c) => c.account.id === uncategorized.id) ? [{ account: uncategorized, depth: 0 }, ...choices] : choices
  const list = `/${administrationId}/products`

  const [file, setFile] = useState<File | null>(null)
  const [dropImage, setDropImage] = useState(false)
  const [title, setTitle] = useState(product?.title ?? '')
  const [identifier, setIdentifier] = useState(product?.identifier ?? '')
  const [description, setDescription] = useState(product?.description ?? '')
  const [sameCheckout, setSameCheckout] = useState(!product?.checkout_description)
  const [checkout, setCheckout] = useState(product?.checkout_description ?? '')
  const [price, setPrice] = useState(priceText(product?.price))
  const [currency, setCurrency] = useState(product?.currency ?? '')
  const [pickCurrency, setPickCurrency] = useState(false)
  const [smart, setSmart] = useState(!!product && !product.tax_rate_id && !!product.vat_rate_type)
  const [productType, setProductType] = useState<string>(product?.product_type ?? 'product')
  const [vatRateType, setVatRateType] = useState<string>(product?.vat_rate_type ?? '')
  const [preview, setPreview] = useState(false)
  const [taxRateId, setTaxRateId] = useState(product ? product.tax_rate_id ?? '' : rates.find((r) => r.active)?.id ?? '')
  const [ledgerAccountId, setLedgerAccountId] = useState(product?.ledger_account_id ?? uncategorized?.id ?? '')
  const [preset, setPreset] = useState(presetOf(product))
  const [frequency, setFrequency] = useState(product?.frequency ? String(product.frequency) : '1')
  const [frequencyType, setFrequencyType] = useState<string>(product?.frequency_type ?? 'month')

  const labels: Record<string, string> = {
    title: t('products.productTitle'), identifier: t('products.reference'), description: t('products.description'),
    checkout_description: t('products.checkoutDescription'), price: t('products.amount'), currency: t('products.currency'),
    tax_rate_id: t('products.vatRate'), product_type: t('products.productType'), vat_rate_type: t('products.vatRateType'),
    ledger_account_id: t('products.category'), frequency: t('products.interval'), frequency_type: t('products.frequency'), image: t('products.productImage'),
  }
  const errors = formErrors(save.error ?? image.error, labels)
  const shownCurrency = currency || administration.data?.currency || 'EUR'
  const shownImage = file ? file.name : !dropImage && product?.image ? product.image.filename : null

  const period = (): Pick<ProductChanges, 'frequency' | 'frequency_type'> => {
    if (preset === '') {
      return { frequency_type: '' as NonNullable<ProductChanges['frequency_type']> }
    }
    const [n, type] = preset === 'other' ? [frequency, frequencyType] : preset.split(':')
    return { frequency_type: type as NonNullable<ProductChanges['frequency_type']>, frequency: Number(n) || 1 }
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: ProductChanges = {
      title: title.trim(),
      identifier: identifier.trim(),
      description: description.trim(),
      checkout_description: sameCheckout ? '' : checkout.trim(),
      price: price.trim(),
      currency: shownCurrency,
      tax_rate_id: smart ? '' : taxRateId,
      product_type: smart ? (productType as ProductChanges['product_type']) : '',
      vat_rate_type: smart ? (vatRateType as ProductChanges['vat_rate_type']) : '',
      ...(ledgerAccountId ? { ledger_account_id: ledgerAccountId } : {}),
      ...period(),
    }
    save.mutate(body, {
      onSuccess: async (saved) => {
        const id = saved.id ?? ''
        if (file || (dropImage && product?.image)) {
          await image.mutateAsync({ id, file }).catch(() => undefined)
        }
        navigate(`${list}/${id}`)
      },
    })
  }

  return (
    <Page title={product ? t('products.editTitle') : t('products.newTitle')} wide>
      <form className="form-sheet form-sheet--stacked" onSubmit={submit} noValidate aria-label={t('products.form')}>
        <Problems messages={errors.general} />
        <FormRow label={labels.image ?? ''} errors={errors.field('image')} help={t('products.imageHelp')}>
          {(control) => (
            <div className="product-drop">
              {shownImage ? (
                <div className="product-drop__file">
                  {!file && product?.image && <img className="product-image product-image--small" src={productImageURL(administrationId, product)} alt="" />}
                  <span>{shownImage}</span>
                  <button type="button" className="btn btn--secondary btn--small" onClick={() => { setFile(null); setDropImage(true) }}>{t('products.removeImage')}</button>
                </div>
              ) : (
                <label className="product-drop__zone">
                  <input
                    {...control}
                    type="file"
                    accept="image/png,image/jpeg,image/gif,image/webp"
                    className="sr-only"
                    onChange={(e) => { setFile(e.target.files?.[0] ?? null); setDropImage(false) }}
                  />
                  <span>{t('products.dropImage')}</span>
                </label>
              )}
            </div>
          )}
        </FormRow>
        <TextInput label={labels.title ?? ''} maxLength={255} value={title} errors={errors.field('title')} onChange={(e) => setTitle(e.target.value)} />
        <TextInput label={labels.identifier ?? ''} maxLength={255} help={t('products.referenceHelp')} value={identifier} errors={errors.field('identifier')} onChange={(e) => setIdentifier(e.target.value)} />
        <TextArea label={labels.description ?? ''} maxLength={10000} value={description} errors={errors.field('description')} onChange={(e) => setDescription(e.target.value)} />
        <CheckboxInput label={t('products.sameCheckout')} checked={sameCheckout} onChange={(e) => setSameCheckout(e.target.checked)} />
        {!sameCheckout && (
          <TextArea label={labels.checkout_description ?? ''} help={t('products.checkoutHelp')} maxLength={10000} value={checkout} errors={errors.field('checkout_description')} onChange={(e) => setCheckout(e.target.value)} />
        )}
        <FormRow label={labels.price ?? ''} errors={errors.field('price')}>
          {({ labelId: _, ...control }) => (
            <div className="product-amount">
              <span className="product-amount__field">
                <input {...control} type="text" inputMode="decimal" placeholder="0.00" value={price} onChange={(e) => setPrice(e.target.value)} />
                <span className="product-amount__currency">{shownCurrency}</span>
              </span>
              {!pickCurrency && <button type="button" className="btn btn--secondary btn--small" onClick={() => setPickCurrency(true)}>{t('products.changeCurrency')}</button>}
            </div>
          )}
        </FormRow>
        {pickCurrency && (
          <SelectInput label={labels.currency ?? ''} width="medium" value={shownCurrency} errors={errors.field('currency')} onChange={(e) => setCurrency(e.target.value)}>
            {currencyOptions(t, shownCurrency).map((o, i) => <option key={`${o.value}-${i}`} value={o.value} disabled={o.disabled}>{o.label}</option>)}
          </SelectInput>
        )}
        <div className="product-vat">
          <p className="form-input__label product-vat__label">{t('products.vatRate')}</p>
          <CheckboxInput label={t('products.smartTax')} checked={smart} onChange={(e) => setSmart(e.target.checked)} />
          {smart ? (
            <>
              <fieldset className="segmented-control" role="radiogroup" aria-labelledby={`${ids}-type`}>
                <legend id={`${ids}-type`} className="sr-only">{labels.product_type}</legend>
                {productTypes.map((p) => (
                  <label key={p} className={`segmented-control__option${productType === p ? ' is-checked' : ''}`}>
                    <input type="radio" name={`${ids}-type`} value={p} checked={productType === p} onChange={() => setProductType(p)} />
                    <span>{t(`products.productTypes.${p}`)}</span>
                  </label>
                ))}
              </fieldset>
              <SelectInput label={labels.vat_rate_type ?? ''} width="medium" value={vatRateType} errors={errors.field('vat_rate_type')} onChange={(e) => setVatRateType(e.target.value)}>
                <option value="">{t('products.selectOption')}</option>
                {vatRateTypes.map((v) => <option key={v} value={v}>{t(`products.vatRateTypes.${v}`)}</option>)}
              </SelectInput>
              <p className="form-input__message form-input__message--help">
                {t('products.smartTaxHelp')}{' '}
                <button type="button" className="product-link-button" aria-expanded={preview} onClick={() => setPreview(!preview)}>{t('products.smartTaxPreview')}</button>
              </p>
              {preview && <SmartTaxPreview productType={productType} vatRateType={vatRateType} />}
            </>
          ) : (
            <SelectInput label={labels.tax_rate_id ?? ''} width="medium" value={taxRateId} errors={errors.field('tax_rate_id')} onChange={(e) => setTaxRateId(e.target.value)}>
              <option value="">{t('products.selectOption')}</option>
              {rates.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
            </SelectInput>
          )}
        </div>
        <SelectInput label={labels.ledger_account_id ?? ''} width="medium" value={ledgerAccountId} errors={errors.field('ledger_account_id')} onChange={(e) => setLedgerAccountId(e.target.value)}>
          {!ledgerAccountId && <option value="">{t('products.chooseCategory')}</option>}
          {categories.map(({ account: a, depth }) => (
            <option key={a.id} value={a.id}>{categoryOption({ account: a, depth })}</option>
          ))}
        </SelectInput>
        <SelectInput label={labels.frequency_type ?? ''} width="small" help={t('products.frequencyHelp')} value={preset} errors={errors.field('frequency_type')} onChange={(e) => setPreset(e.target.value)}>
          {presets.map((p) => <option key={p} value={p}>{t(`products.presets.${presetName[p]}`)}</option>)}
          <option value="other">{t('products.presets.other')}</option>
        </SelectInput>
        {preset === 'other' && (
          <div className="field-pair product-period">
            <TextInput label={labels.frequency ?? ''} type="number" inputMode="numeric" min={1} max={1000} width="small" value={frequency} errors={errors.field('frequency')} onChange={(e) => setFrequency(e.target.value)} />
            <SelectInput label={t('products.periodUnit')} width="small" value={frequencyType} onChange={(e) => setFrequencyType(e.target.value)}>
              {productFrequencyTypes.map((f) => <option key={f} value={f}>{t(`products.units.${f}`)}</option>)}
            </SelectInput>
          </div>
        )}
        <div className="form-sheet__actions form-sheet__actions--center">
          <button className="btn btn--primary" disabled={save.isPending || image.isPending}>{t('products.saveShort')}</button>
          <ActionLink className="btn btn--secondary" to={product ? `${list}/${product.id}` : list}>{t('common.cancel')}</ActionLink>
        </div>
      </form>
    </Page>
  )
}

/**
 * Which VAT the smart tax selection gives, by where the customer is, as
 * Moneybird's preview: Dutch customers pay the Dutch rate of the type, EU
 * businesses are reverse charged, EU consumers of digital services pay
 * their own country's rate, and customers outside the EU pay none.
 */
function SmartTaxPreview({ productType, vatRateType }: { productType: string; vatRateType: string }) {
  const { t } = useTranslation()
  const dutch = vatRateType === 'reduced' ? '9%' : vatRateType === 'standard' ? '21%' : '-'
  const rows: [string, string][] = [
    [t('products.preview.nl'), dutch],
    [t('products.preview.euBusiness'), t('products.preview.reverseCharge')],
    [t('products.preview.euConsumer'), productType === 'digital_service' ? t('products.preview.oss') : dutch],
    [t('products.preview.outside'), '0%'],
  ]
  return (
    <table className="data-table product-preview" aria-label={t('products.smartTaxPreviewTitle')}>
      <thead>
        <tr><th scope="col">{t('products.preview.customer')}</th><th scope="col">{t('products.preview.rate')}</th></tr>
      </thead>
      <tbody>
        {rows.map(([who, rate]) => <tr key={who}><th scope="row">{who}</th><td>{rate}</td></tr>)}
      </tbody>
    </table>
  )
}
