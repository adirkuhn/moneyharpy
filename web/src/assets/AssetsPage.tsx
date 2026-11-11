import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { FilterPanel } from '../list/FilterPanel'
import { type Asset, type AssetView, assetViews, perPage, useAssets } from './queries'

/** The administration's fixed assets, active ones unless all are asked for, and paged (/assets). */
export function AssetsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const view: AssetView = params.get('active') === 'false' ? 'all' : 'active'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const assets = useAssets(administrationId, view, page)
  const administration = useAdministration(administrationId)
  const currency = administration.data?.currency ?? 'EUR'
  const base = `/${administrationId}/assets`
  const shown = assets.data?.assets ?? []
  const quick = assetViews.map((v) => ({ label: t(`assets.views.${v}`), to: v === 'active' ? base : `${base}?active=false`, current: v === view }))

  return (
    <Page wide title={t('assets.title')} lead={<FilterPanel label={t(`assets.views.${view}`)} title={t('assets.filters')} quick={quick} />}>
      {!assets.data ? (
        assets.error ? <p className="empty">{t('assets.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{view === 'active' && page === 1 ? t('assets.empty') : t('assets.noMatches')}</p>
      ) : (
        <DocumentTable<Asset>
          label={t('assets.title')}
          rows={shown}
          rowKey={(a) => String(a.id ?? '')}
          href={(a) => `${base}/${a.id}`}
          total={assets.data.total}
          perPage={perPage}
          columns={[
            { key: 'name', header: t('assets.name'), link: true, cell: (a) => a.name },
            { key: 'date', header: t('assets.purchaseDate'), kind: 'date', cell: (a) => listDate(a.purchase_date) },
            { key: 'purchase', header: t('assets.purchaseValue'), kind: 'amount', cell: (a) => formatAmount(String(a.purchase_value ?? 0), currency, i18n.language) },
            { key: 'current', header: t('assets.currentValue'), kind: 'amount', cell: (a) => formatAmount(String(a.current_value ?? 0), currency, i18n.language) },
            { key: 'state', header: t('assets.status'), kind: 'state', cell: (a) => <AssetBadge asset={a} /> },
          ]}
        />
      )}
    </Page>
  )
}

export function AssetBadge({ asset }: { asset: Asset }) {
  const { t } = useTranslation()
  if (asset.disposal) {
    return <span className="badge badge--archived">{t('assets.disposed')}</span>
  }
  if (!asset.value_change_plan) {
    return <span className="badge badge--active">{t('assets.noPlan')}</span>
  }
  return <span className={`badge badge--${asset.value_change_plan.active ? 'active' : 'archived'}`}>{t(asset.value_change_plan.active ? 'assets.active' : 'assets.inactive')}</span>
}
