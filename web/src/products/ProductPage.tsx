import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { bookable, formatAmount } from '../banking/queries'
import { type LedgerAccount, type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { ConfirmPanel, DetailBlock, DetailList, type DetailRow } from '../detail/Detail'
import { FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type MenuItem, MoreMenu } from '../list/ListActions'
import {
  type Product, type ProductChanges, productFrequency, productFrequencyTypes, productName, useDeleteProduct, useProduct, useSaveProduct,
} from './queries'

/** One product (/products/:productId), or a new one (/products/new). */
export function ProductPage() {
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
  if (!current) {
    return <NewProduct administrationId={administrationId} rates={usable} accounts={accounts.data} />
  }
  return <ProductView key={current.id} administrationId={administrationId} product={current} rates={usable} accounts={accounts.data} />
}

type Choices = { rates: TaxRate[]; accounts: LedgerAccount[] }

function NewProduct({ administrationId, rates, accounts }: { administrationId: string } & Choices) {
  const { t } = useTranslation()
  return (
    <Page title={t('products.newTitle')} back={{ to: `/${administrationId}/products`, label: t('products.backToList') }}>
      <ProductForm administrationId={administrationId} product={null} rates={rates} accounts={accounts} />
    </Page>
  )
}

function ProductView({ administrationId, product, rates, accounts }: { administrationId: string; product: Product } & Choices) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveProduct(administrationId, product.id)
  const remove = useDeleteProduct(administrationId, product.id ?? '')
  const list = `/${administrationId}/products`
  const [confirming, setConfirming] = useState(false)
  const [kept, setKept] = useState(false)

  const more: MenuItem[] = [
    product.active
      ? { label: t('products.deactivate'), disabled: save.isPending, onSelect: () => save.mutate({ active: false }) }
      : { label: t('products.reactivate'), disabled: save.isPending, onSelect: () => save.mutate({ active: true }) },
    { label: t('products.delete'), danger: true, onSelect: () => setConfirming(true) },
  ]
  const account = accounts.find((a) => a.id === product.ledger_account_id)
  const rows: DetailRow[] = [
    [t('products.status'), <span key="state" className={`badge badge--${product.active ? 'active' : 'inactive'}`}>{t(`products.states.${product.active ? 'active' : 'inactive'}`)}</span>],
    [t('products.price'), formatAmount(product.price, product.currency ?? 'EUR', i18n.language)],
    [t('products.frequency'), productFrequency(product, t)],
    [t('products.vatRate'), rates.find((r) => r.id === product.tax_rate_id)?.name],
    [t('products.category'), account?.name],
    [t('products.identifier'), product.identifier],
  ]

  return (
    <Page
      title={productName(product, t)}
      subtitle={product.identifier ?? undefined}
      back={{ to: list, label: t('products.backToList') }}
      actions={<MoreMenu items={more} />}
    >
      <Problems messages={formErrors(remove.error ?? save.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('products.confirmDeleteText')}
          confirmLabel={t('products.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, {
            onSuccess: (keptAs) => {
              setConfirming(false)
              if (keptAs) {
                setKept(true)
              } else {
                navigate(list)
              }
            },
          })}
          onCancel={() => setConfirming(false)}
        />
      )}
      {kept && <p className="notice" role="status">{t('products.keptNotice')}</p>}
      {!product.active && !kept && <p className="notice">{t('products.inactiveNotice')}</p>}
      <DetailBlock title={t('products.detailsTitle')}>
        <DetailList rows={rows} />
      </DetailBlock>
      <DetailBlock title={t('products.editTitle')}>
        <ProductForm key={product.updated_at} administrationId={administrationId} product={product} rates={rates} accounts={accounts} />
      </DetailBlock>
    </Page>
  )
}

/** Shows "100.0" from the server as "100" and keeps "12.5" as it is. */
const priceText = (p: string | undefined) => (p === undefined || p === '' ? '' : String(Number(p)))

type FormProps = { administrationId: string; product: Product | null } & Choices

/** What a product is, what it costs and where it is booked. */
function ProductForm({ administrationId, product, rates, accounts }: FormProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveProduct(administrationId, product?.id)
  const administration = useAdministration(administrationId)
  const groups = bookable(accounts, 'sales_invoice')

  const [title, setTitle] = useState(product?.title ?? '')
  const [description, setDescription] = useState(product?.description ?? '')
  const [identifier, setIdentifier] = useState(product?.identifier ?? '')
  const [price, setPrice] = useState(priceText(product?.price))
  const [currency, setCurrency] = useState(product?.currency ?? '')
  const [taxRateId, setTaxRateId] = useState(product ? product.tax_rate_id ?? '' : rates.find((r) => r.active)?.id ?? '')
  const [ledgerAccountId, setLedgerAccountId] = useState(product?.ledger_account_id ?? '')
  const [frequency, setFrequency] = useState(product?.frequency ? String(product.frequency) : '')
  const [frequencyType, setFrequencyType] = useState<string>(product?.frequency_type ?? '')

  const labels: Record<string, string> = {
    title: t('products.productTitle'), description: t('products.description'), identifier: t('products.identifier'), price: t('products.price'),
    currency: t('products.currency'), tax_rate_id: t('products.vatRate'), ledger_account_id: t('products.category'),
    frequency: t('products.interval'), frequency_type: t('products.frequency'),
  }
  const errors = formErrors(save.error, labels)
  const shownCurrency = currency || administration.data?.currency || 'EUR'

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: ProductChanges = {
      title: title.trim(),
      description: description.trim(),
      identifier: identifier.trim(),
      price: price.trim(),
      currency: shownCurrency.trim().toUpperCase(),
      ...(taxRateId ? { tax_rate_id: taxRateId } : {}),
      ...(ledgerAccountId ? { ledger_account_id: ledgerAccountId } : {}),
      frequency_type: frequencyType as NonNullable<ProductChanges['frequency_type']>,
      ...(frequencyType ? { frequency: Number(frequency) || frequency || 1 } : {}),
    }
    save.mutate(body, {
      onSuccess: (saved) => {
        if (!product) {
          navigate(`/${administrationId}/products/${saved.id}`)
        }
      },
    })
  }

  return (
    <form className="form-sheet" onSubmit={submit} noValidate aria-label={t('products.form')}>
      <Problems messages={errors.general} />
      <FormSection>
        <TextInput label={labels.title ?? ''} maxLength={255} value={title} errors={errors.field('title')} onChange={(e) => setTitle(e.target.value)} />
        <TextArea label={labels.description ?? ''} maxLength={10000} help={t('products.descriptionHelp')} value={description} errors={errors.field('description')} onChange={(e) => setDescription(e.target.value)} />
        <TextInput label={labels.identifier ?? ''} maxLength={255} width="medium" help={t('products.identifierHelp')} value={identifier} errors={errors.field('identifier')} onChange={(e) => setIdentifier(e.target.value)} />
      </FormSection>
      <FormSection title={t('products.priceSection')}>
        <TextInput label={labels.price ?? ''} inputMode="decimal" width="small" required value={price} errors={errors.field('price')} onChange={(e) => setPrice(e.target.value)} />
        <TextInput label={labels.currency ?? ''} width="small" maxLength={3} value={shownCurrency} errors={errors.field('currency')} onChange={(e) => setCurrency(e.target.value)} />
        <SelectInput label={labels.tax_rate_id ?? ''} width="medium" value={taxRateId} errors={errors.field('tax_rate_id')} onChange={(e) => setTaxRateId(e.target.value)}>
          <option value="">{t('products.noVatRate')}</option>
          {rates.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
        </SelectInput>
        <SelectInput label={labels.ledger_account_id ?? ''} width="medium" required value={ledgerAccountId} errors={errors.field('ledger_account_id')} onChange={(e) => setLedgerAccountId(e.target.value)}>
          <option value="">{t('products.chooseCategory')}</option>
          {groups.map((g) => (
            <optgroup key={g.type} label={t(`bookkeeping.accountTypes.${g.type}`)}>
              {g.rows.map(({ account: a, depth }) => (
                <option key={a.id} value={a.id}>{`${' '.repeat(depth)}${a.account_id ? `${a.account_id} ` : ''}${a.name}`}</option>
              ))}
            </optgroup>
          ))}
        </SelectInput>
      </FormSection>
      <FormSection title={t('products.subscription')}>
        <SelectInput label={labels.frequency_type ?? ''} width="medium" help={t('products.frequencyHelp')} value={frequencyType} errors={errors.field('frequency_type')} onChange={(e) => setFrequencyType(e.target.value)}>
          <option value="">{t('products.once')}</option>
          {productFrequencyTypes.map((f) => <option key={f} value={f}>{t(`recurring.types.${f}`)}</option>)}
        </SelectInput>
        {frequencyType && (
          <TextInput label={labels.frequency ?? ''} type="number" inputMode="numeric" min={1} max={1000} width="small" placeholder="1" value={frequency} errors={errors.field('frequency')} onChange={(e) => setFrequency(e.target.value)} />
        )}
      </FormSection>
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={save.isPending}>{t('products.save')}</button>
        {!product && <Link className="btn btn--secondary" to={`/${administrationId}/products`}>{t('common.cancel')}</Link>}
      </div>
    </form>
  )
}
