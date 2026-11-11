import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { ConfirmPanel, DetailBlock, DetailList, type DetailRow } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { AssetBadge } from './AssetsPage'
import { type Asset, useAsset, useDeleteAsset } from './queries'

/** One fixed asset (/assets/:assetId): its value, depreciation plan, disposal and value changes. */
export function AssetPage() {
  const { administrationId = '', assetId = '' } = useParams()
  const asset = useAsset(administrationId, assetId)
  if (asset.error instanceof ApiError && asset.error.status < 500) {
    return <NotFoundPage />
  }
  if (!asset.data) {
    return <Loading />
  }
  return <AssetView key={String(asset.data.id)} administrationId={administrationId} asset={asset.data} />
}

function AssetView({ administrationId, asset }: { administrationId: string; asset: Asset }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const administration = useAdministration(administrationId)
  const remove = useDeleteAsset(administrationId, String(asset.id ?? ''))
  const [confirming, setConfirming] = useState(false)
  const list = `/${administrationId}/assets`
  const currency = administration.data?.currency ?? 'EUR'
  const money = (amount: string | number | null | undefined) => formatAmount(String(amount ?? '0'), currency, i18n.language)
  const date = (d: string | null | undefined) => d && new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  const plan = asset.value_change_plan
  const disposal = asset.disposal
  const rows: DetailRow[] = [
    [t('assets.status'), <AssetBadge key="state" asset={asset} />],
    [t('assets.purchaseDate'), date(asset.purchase_date)],
    [t('assets.purchaseValue'), money(asset.purchase_value)],
    [t('assets.calculatedPurchaseValue'), asset.calculated_purchase_value !== asset.purchase_value && money(asset.calculated_purchase_value)],
    [t('assets.currentValue'), money(asset.current_value)],
    [t('assets.lifespan'), plan && t('assets.years', { count: plan.lifespan_in_years ?? 0 })],
    [t('assets.residualValue'), plan && money(plan.residual_value)],
    [t('assets.monthlyDepreciation'), plan && money(plan.monthly_depreciation)],
    [t('assets.reservePurchase'), asset.reinvestment_reserve_purchase && money(asset.reinvestment_reserve_purchase.amount)],
    [t('assets.disposal'), disposal && `${date(disposal.date)} — ${t(`assets.reasons.${disposal.reason ?? 'out_of_use'}`)}`],
    [t('assets.reserveSale'), disposal?.reinvestment_reserve_sale && money(disposal.reinvestment_reserve_sale.amount)],
  ]

  return (
    <Page
      title={asset.name ?? ''}
      back={{ to: list, label: t('assets.backTo') }}
      actions={<MoreMenu items={[{ label: t('assets.delete'), danger: true, onSelect: () => setConfirming(true) }]} />}
    >
      <Problems messages={formErrors(remove.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('assets.confirmDelete')}
          confirmLabel={t('assets.delete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      <DetailBlock title={t('assets.details')}>
        <DetailList rows={rows} />
      </DetailBlock>
      <DetailBlock title={t('assets.valueChanges')}>
        {(asset.value_changes ?? []).length === 0 ? (
          <p className="empty">{t('assets.noValueChanges')}</p>
        ) : (
          <table className="data-table" aria-label={t('assets.valueChanges')}>
            <thead>
              <tr>
                <th>{t('assets.date')}</th>
                <th>{t('assets.type')}</th>
                <th>{t('assets.description')}</th>
                <th className="transactions__amount">{t('assets.amount')}</th>
              </tr>
            </thead>
            <tbody>
              {(asset.value_changes ?? []).map((c, i) => (
                <tr key={`${c.date}-${i}`}>
                  <td>{date(c.date)}</td>
                  <td>{t(`assets.types.${c.type ?? 'manual'}`)}</td>
                  <td>{c.description}</td>
                  <td className="transactions__amount">{money(c.amount)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </DetailBlock>
    </Page>
  )
}
