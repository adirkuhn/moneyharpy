import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { type LedgerAccount, type TaxRate, useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { ConfirmPanel, DetailList, type DetailRow } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type MenuItem, MoreMenu } from '../list/ListActions'
import { type Product, productFrequency, productImageURL, productName, useDeleteProduct, useProduct } from './queries'
import './products.css'

/**
 * One product (/products/:productId), after Moneybird's: what it is, its
 * amount, VAT and category, how often it is billed and its image, with Edit
 * and, under ⋯, Remove product.
 */
export function ProductPage() {
  const { administrationId = '', productId = '' } = useParams()
  const product = useProduct(administrationId, productId)
  const rates = useTaxRates(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  if (product.error instanceof ApiError && product.error.status < 500) {
    return <NotFoundPage />
  }
  if (!product.data || !rates.data || !accounts.data) {
    return <Loading />
  }
  return <ProductView administrationId={administrationId} product={product.data} rates={rates.data} accounts={accounts.data} />
}

type ViewProps = { administrationId: string; product: Product; rates: TaxRate[]; accounts: LedgerAccount[] }

function ProductView({ administrationId, product, rates, accounts }: ViewProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const remove = useDeleteProduct(administrationId, product.id ?? '')
  const list = `/${administrationId}/products`
  const [confirming, setConfirming] = useState(false)
  const [kept, setKept] = useState(false)

  const more: MenuItem[] = [{ label: t('products.remove'), danger: true, onSelect: () => setConfirming(true) }]
  const smart = !product.tax_rate_id && !!product.vat_rate_type
  const rows: DetailRow[] = [
    [t('products.productTitle'), product.title || '-'],
    [t('products.description'), product.description || '-'],
    [t('products.amount'), formatAmount(product.price, product.currency ?? 'EUR', i18n.language)],
    [t('products.productType'), product.product_type ? t(`products.productTypes.${product.product_type}`) : undefined],
    smart
      ? [t('products.tax_rate_type'), t(`products.vatRateTypes.${product.vat_rate_type}`)]
      : [t('products.vatRate'), rates.find((r) => r.id === product.tax_rate_id)?.name ?? '-'],
    [t('products.category'), accounts.find((a) => a.id === product.ledger_account_id)?.name],
    [t('products.frequency'), productFrequency(product, t) || t('products.noPeriodSelected')],
    [t('products.image'), product.image ? <img key="image" className="product-image" src={productImageURL(administrationId, product)} alt={product.image.filename} /> : '-'],
  ]

  return (
    <Page
      title={productName(product, t)}
      back={{ to: list, label: t('products.backToList') }}
      actions={
        <>
          <Link
            className="btn btn--accent"
            to={`/${administrationId}/payment_links/${product.id}/edit?return_to=${encodeURIComponent(`${list}/${product.id}`)}`}
          >
            {t('products.createPaymentLink')}
          </Link>
          <Link className="btn btn--secondary btn--with-icon" to={`${list}/${product.id}/edit`}>
            <ListIcon kind="edit" />
            <span>{t('products.edit')}</span>
          </Link>
          <MoreMenu items={more} />
        </>
      }
    >
      <Problems messages={formErrors(remove.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('products.confirmRemoveText')}
          confirmLabel={t('products.remove')}
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
      <section className="detail-block product-details" aria-label={t('products.detailsTitle')}>
        <DetailList rows={rows} />
      </section>
    </Page>
  )
}
