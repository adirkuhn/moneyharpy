import { ActionLink } from '../layout/ActionLink'
import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { useLedgerAccounts, useTaxRates } from '../bookkeeping/queries'
import { InfoIcon, ListIcon, SearchIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable } from '../list/DocumentTable'
import { GettingStarted } from '../list/GettingStarted'
import { exportProducts } from './exportProducts'
import { perPage, type Product, productName, useProducts } from './queries'
import { formatUnitPrice } from '../invoices/totals'

type Order = 'none' | 'ascending' | 'descending'

/**
 * The administration's products, after Moneybird's: the title, amount, VAT
 * rate and category of each, searchable and sortable by amount, with the
 * introduction until the first product is added.
 */
export function ProductsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const [query, setQuery] = useState(params.get('q') ?? '')
  const products = useProducts(administrationId, { query: params.get('q') ?? '', all: false, page })
  const rates = useTaxRates(administrationId).data ?? []
  const accounts = useLedgerAccounts(administrationId).data ?? []
  const [info, setInfo] = useState<boolean | null>(null)
  const [order, setOrder] = useState<Order>('none')
  const [exporting, setExporting] = useState(false)
  const introId = useId()
  const base = `/${administrationId}/products`
  const loaded = products.data?.products ?? []
  const firstUse = products.data !== undefined && loaded.length === 0 && !params.get('q') && page === 1
  const introShown = info ?? firstUse
  const shown = order === 'none' ? loaded : [...loaded].sort((a, b) => (Number(a.price) - Number(b.price)) * (order === 'ascending' ? 1 : -1))

  // Searching waits for a pause in typing, so each key does not ask the server.
  useEffect(() => {
    if (query === (params.get('q') ?? '')) {
      return
    }
    const timer = setTimeout(() => {
      const next = new URLSearchParams(params)
      if (query.trim()) {
        next.set('q', query.trim())
      } else {
        next.delete('q')
      }
      next.delete('page')
      setParams(next, { replace: true })
    }, 250)
    return () => clearTimeout(timer)
  })

  const rateName = (p: Product) => rates.find((r) => r.id === p.tax_rate_id)?.name ?? ''
  const categoryName = (p: Product) => accounts.find((a) => a.id === p.ledger_account_id)?.name ?? ''
  const download = () => {
    setExporting(true)
    void exportProducts(administrationId, { rate: rateName, category: categoryName }, t, i18n.language).finally(() => setExporting(false))
  }

  return (
    <Page
      wide
      title={t('products.title')}
      actions={
        <>
          <button
            type="button"
            className={`btn btn--secondary btn--icon${introShown ? ' is-active' : ''}`}
            aria-label={t('list.moreInfo')}
            title={t('list.moreInfo')}
            aria-expanded={introShown}
            aria-controls={introId}
            onClick={() => setInfo(!introShown)}
          >
            <InfoIcon />
          </button>
          <button type="button" className="btn btn--secondary btn--icon" aria-label={t('products.export')} title={t('products.export')} disabled={exporting} onClick={download}>
            <ListIcon kind="download" />
          </button>
          <ActionLink className="btn btn--accent btn--with-icon" to={`${base}/new`}>
            <ListIcon kind="plus" />
            <span>{t('products.addShort')}</span>
          </ActionLink>
        </>
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('products.intro.title')}
          text={t('products.intro.text')}
          steps={[
            { icon: 'plus', title: t('products.intro.add'), text: t('products.intro.addText') },
            { icon: 'file', title: t('products.intro.invoice'), text: t('products.intro.invoiceText') },
            { icon: 'link', title: t('products.intro.online'), text: t('products.intro.onlineText') },
          ]}
        />
      )}
      {!firstUse && products.data && (
        <div className="list-search list-search--full" role="search" aria-label={t('products.search')}>
          <SearchIcon />
          <input type="search" aria-label={t('products.search')} placeholder={t('products.search')} value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
      )}
      {!products.data ? (
        products.error ? <p className="empty">{t('products.loadError')}</p> : <Loading />
      ) : shown.length === 0 && !firstUse ? (
        <p className="empty">{t('products.noMatches')}</p>
      ) : (
        <div className="product-list">
          <DocumentTable<Product>
            label={t('products.title')}
            rows={shown}
            rowKey={(p) => p.id ?? ''}
            href={(p) => `${base}/${p.id}`}
            total={products.data.total}
            perPage={perPage}
            footer={firstUse && (
              <ActionLink className="document-table__add" to={`${base}/new`}>
                <ListIcon kind="plus" />
                {t('products.add')}
              </ActionLink>
            )}
            columns={[
              { key: 'title', header: t('products.productTitle'), link: true, cell: (p) => productName(p, t) },
              {
                key: 'price', header: t('products.amount'), kind: 'amount', cell: (p) => formatUnitPrice(p.price, p.currency ?? 'EUR', i18n.language),
                sort: { direction: order, onToggle: () => setOrder(order === 'ascending' ? 'descending' : 'ascending') },
              },
              { key: 'rate', header: t('products.vatRate'), cell: rateName },
              { key: 'category', header: t('products.category'), cell: categoryName },
            ]}
          />
        </div>
      )}
    </Page>
  )
}
