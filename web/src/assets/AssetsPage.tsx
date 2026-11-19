import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { formatAmount } from '../banking/queries'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable } from '../list/DocumentTable'
import { GettingStarted } from '../list/GettingStarted'
import { type Asset, type AssetView, perPage, useAssetCount, useAssets } from './queries'
import './assets.css'

/**
 * The administration's fixed assets, after Moneybird's list: the active ones,
 * with their category, commissioning date, current value and status, and
 * below them how many are deactivated and a way to show those too (/assets).
 */
export function AssetsPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const view: AssetView = params.get('active') === 'false' ? 'all' : 'active'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const assets = useAssets(administrationId, view, page)
  const active = useAssetCount(administrationId, 'active')
  const all = useAssetCount(administrationId, 'all')
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId).data ?? []
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const currency = administration.data?.currency ?? 'EUR'
  const base = `/${administrationId}/assets`
  const shown = assets.data?.assets ?? []
  const firstUse = all.data === 0
  const introShown = info ?? firstUse
  const deactivated = all.data !== undefined && active.data !== undefined ? all.data - active.data : undefined

  const footer = firstUse ? (
    <Link className="document-table__add" to={`${base}/new`}>
      <ListIcon kind="plus" />
      {t('assets.add')}
    </Link>
  ) : deactivated !== undefined && (
    <span className="asset-list__deactivated">
      {t('assets.deactivatedCount', { count: deactivated })}{' '}
      {view === 'active'
        ? deactivated > 0 && <Link to={`${base}?active=false`}>{t('assets.showDeactivated')}</Link>
        : <Link to={base}>{t('assets.hideDeactivated')}</Link>}
    </span>
  )

  return (
    <Page
      wide
      title={t('assets.title')}
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
          <Link className="btn btn--accent btn--with-icon" to={`${base}/new`}>
            <ListIcon kind="plus" />
            <span>{t('assets.addShort')}</span>
          </Link>
        </>
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('assets.intro.title')}
          text={t('assets.intro.text')}
          steps={[
            { icon: 'plus', title: t('assets.intro.add'), text: t('assets.intro.addText') },
            { icon: 'chart', title: t('assets.intro.spread'), text: t('assets.intro.spreadText') },
            { icon: 'scale', title: t('assets.intro.overview'), text: t('assets.intro.overviewText') },
          ]}
        />
      )}
      {!assets.data ? (
        assets.error ? <p className="empty">{t('assets.loadError')}</p> : <Loading />
      ) : (
        <div className="asset-list">
          <DocumentTable<Asset>
            label={t('assets.title')}
            rows={shown}
            rowKey={(a) => String(a.id ?? '')}
            href={(a) => `${base}/${a.id}`}
            total={assets.data.total}
            perPage={perPage}
            footer={footer}
            columns={[
              { key: 'name', header: t('assets.name'), link: true, cell: (a) => a.name },
              {
                key: 'category', header: t('assets.category'),
                cell: (a) => <span className="asset-category">{accounts.find((l) => l.id === a.ledger_account_id)?.name ?? ''}</span>,
              },
              { key: 'date', header: t('assets.commissioningDate'), kind: 'date', cell: (a) => a.purchase_date },
              { key: 'current', header: t('assets.currentValue'), kind: 'amount', cell: (a) => formatAmount(String(a.current_value ?? 0), currency, i18n.language) },
              { key: 'state', header: t('assets.status'), kind: 'state', cell: (a) => <AssetBadge asset={a} /> },
            ]}
          />
        </div>
      )}
    </Page>
  )
}

/** The asset's state, as Moneybird's label: active while in use, else how it left. */
export function AssetBadge({ asset }: { asset: Asset }) {
  const { t } = useTranslation()
  if (asset.disposal) {
    return <span className="badge badge--archived">{t(`assets.reasons.${asset.disposal.reason ?? 'out_of_use'}`)}</span>
  }
  if (asset.value_change_plan && !asset.value_change_plan.active) {
    return <span className="badge badge--archived">{t('assets.depreciated')}</span>
  }
  return <span className="badge badge--open">{t('assets.active')}</span>
}
