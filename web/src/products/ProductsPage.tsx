import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { formatAmount } from '../banking/queries'
import { SearchIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import { perPage, type Product, productFrequency, productName, type ProductSearch, useProducts } from './queries'

/** The administration's products, searchable, active ones unless all are asked for, and paged (/products). */
export function ProductsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const all = params.get('active') === 'false'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const [query, setQuery] = useState(params.get('q') ?? '')
  const search: ProductSearch = { query: params.get('q') ?? '', all, page }
  const products = useProducts(administrationId, search)
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const base = `/${administrationId}/products`
  const shown = products.data?.products ?? []
  const firstUse = products.data !== undefined && shown.length === 0 && !search.query && !all && page === 1
  const introShown = info ?? firstUse

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

  const quick = [
    { label: t('products.views.active'), to: base, current: !all },
    { label: t('products.views.all'), to: `${base}?active=false`, current: all },
  ]

  return (
    <Page
      wide
      title={t('products.title')}
      lead={<FilterPanel label={t(`products.views.${all ? 'all' : 'active'}`)} title={t('products.filters')} quick={quick} />}
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          exports={[]}
          primary={{ to: `${base}/new`, label: t('products.add') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('products.intro.title')}
          text={t('products.intro.text')}
          steps={[
            { icon: 'plus', title: t('products.intro.add'), text: t('products.intro.addText') },
            { icon: 'coins', title: t('products.intro.price'), text: t('products.intro.priceText') },
            { icon: 'file', title: t('products.intro.invoice'), text: t('products.intro.invoiceText') },
          ]}
        />
      )}
      <div className="list-search" role="search" aria-label={t('products.search')}>
        <SearchIcon />
        <input type="search" aria-label={t('products.search')} placeholder={t('products.search')} value={query} onChange={(e) => setQuery(e.target.value)} />
      </div>
      {!products.data ? (
        products.error ? <p className="empty">{t('products.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{firstUse ? t('products.empty') : t('products.noMatches')}</p>
      ) : (
        <DocumentTable<Product>
          label={t('products.title')}
          rows={shown}
          rowKey={(p) => p.id ?? ''}
          href={(p) => `${base}/${p.id}`}
          total={products.data.total}
          perPage={perPage}
          columns={[
            { key: 'title', header: t('products.name'), link: true, cell: (p) => productName(p, t) },
            { key: 'identifier', header: t('products.identifier'), kind: 'extra', cell: (p) => p.identifier },
            { key: 'frequency', header: t('products.frequency'), kind: 'extra', cell: (p) => productFrequency(p, t) },
            { key: 'price', header: t('products.price'), kind: 'amount', cell: (p) => formatAmount(p.price, p.currency ?? 'EUR', i18n.language) },
            {
              key: 'state', header: t('products.status'), kind: 'state',
              cell: (p) => <span className={`badge badge--${p.active ? 'active' : 'inactive'}`}>{t(`products.states.${p.active ? 'active' : 'inactive'}`)}</span>,
            },
          ]}
        />
      )}
    </Page>
  )
}
