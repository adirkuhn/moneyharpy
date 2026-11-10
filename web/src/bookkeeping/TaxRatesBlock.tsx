import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Block } from '../settings/Block'
import { type TaxRate, type TaxRateType, taxRateTypes, useTaxRates } from './queries'

/** How a rate reads in a list: "VAT 21%", or "No VAT on invoice and VAT 21%" when it is not printed. */
export function rateLabel(t: TFunction, rate: TaxRate) {
  const percentage = String(rate.percentage ?? 0)
  return rate.show_tax ? t('bookkeeping.rate', { percentage }) : t('bookkeeping.rateHidden', { percentage })
}

/** VAT rates per kind of document, with the VAT return boxes they count in. */
export function TaxRatesBlock({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const rates = useTaxRates(administrationId)
  const base = `/${administrationId}`
  return (
    <Block
      id="vat-rates"
      title={t('bookkeeping.parts.rates')}
      subtitle={t('bookkeeping.ratesSubtitle')}
      action={<Link className="btn btn--accent" to={`${base}/tax_rates/new`}>{t('bookkeeping.addRate')}</Link>}
    >
      {taxRateTypes.map((type) => (
        <RateTable key={type} administrationId={administrationId} type={type} rates={rates.data?.filter((r) => r.tax_rate_type === type) ?? []} />
      ))}
    </Block>
  )
}

function RateTable({ administrationId, type, rates }: { administrationId: string; type: TaxRateType; rates: TaxRate[] }) {
  const { t } = useTranslation()
  const title = t(`bookkeeping.rateTypes.${type}`)
  if (rates.length === 0) {
    return (
      <div className="table-card">
        <p className="table-card__title">{title}</p>
        <p className="empty table-card__empty">{t('bookkeeping.noRates')}</p>
      </div>
    )
  }
  return (
    <table className="data-table" aria-label={title}>
      <thead>
        <tr>
          <th scope="col">{title}</th>
          <th scope="col" className="data-table__extra">{t('bookkeeping.rateColumn')}</th>
          <th scope="col" className="data-table__extra">{t('bookkeeping.reportColumn')}</th>
        </tr>
      </thead>
      <tbody>
        {rates.map((r) => (
          <tr key={r.id} className={r.active ? undefined : 'is-inactive'}>
            <td>
              <Link to={`/${administrationId}/tax_rates/${r.id}`}>{r.name}</Link>
              {!r.active && <span className="hint"> · {t('bookkeeping.inactive')}</span>}
            </td>
            <td className="data-table__extra">{rateLabel(t, r)}</td>
            <td className="data-table__extra">{r.tax_report_boxes.length ? r.tax_report_boxes.map((b) => b.replace(/^[A-Z]{2}\//, '')).join(', ') : '-'}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
