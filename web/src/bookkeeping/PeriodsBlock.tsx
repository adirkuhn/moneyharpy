import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { errorMessages } from '../api/client'
import { Block } from '../settings/Block'
import { type BookkeepingSettings, useUpdateBookkeepingSettings } from './queries'

type Period = BookkeepingSettings['vat_documents_period']
type Field = 'vat_documents_period' | 'intra_community_transactions_declaration_period'
const periods: Period[] = ['month', 'quarter', 'year']

/** How often the VAT return and the ICP declaration are filed. */
export function PeriodsBlock({ administrationId, settings, country }: { administrationId: string; settings: BookkeepingSettings; country: string }) {
  const { t } = useTranslation()
  const update = useUpdateBookkeepingSettings(administrationId)
  const [pending, setPending] = useState<{ field: Field; period: Period } | null>(null)

  const group = (field: Field, label: string) => (
    <fieldset className="segmented">
      <legend className="segmented__legend">{label}</legend>
      <div className="segmented__options">
        {periods.map((p) => {
          const current = (pending?.field === field ? pending.period : settings[field]) === p
          return (
            <label key={p} className={`segmented__option${current ? ' is-checked' : ''}`}>
              <input type="radio" name={field} value={p} checked={current} disabled={update.isPending} onChange={() => setPending(p === settings[field] ? null : { field, period: p })} />
              {t(`bookkeeping.periods.${p}`)}
            </label>
          )
        })}
      </div>
    </fieldset>
  )

  return (
    <Block id="declaration-period" title={t('bookkeeping.parts.periods')} subtitle={t('bookkeeping.periodsSubtitle')}>
      <div className="card card-stack">
        {group('vat_documents_period', t('bookkeeping.vatPeriod'))}
        {group('intra_community_transactions_declaration_period', t('bookkeeping.icpPeriod'))}
        {pending && (
          <div className="confirm-row" role="alert">
            <p>{t('bookkeeping.confirmPeriod', { period: t(`bookkeeping.periods.${pending.period}`) })}</p>
            <div className="confirm-row__buttons">
              <button type="button" className="btn btn--accent btn--small" disabled={update.isPending} onClick={() => update.mutate({ [pending.field]: pending.period }, { onSuccess: () => setPending(null) })}>
                {t('bookkeeping.change')}
              </button>
              <button type="button" className="btn btn--secondary btn--small" onClick={() => setPending(null)}>{t('common.cancel')}</button>
            </div>
          </div>
        )}
        <Problems messages={errorMessages(update.error, {})} />
        {country === 'NL' && (
          <p className="card__footer">
            {t('bookkeeping.periodHelp')}{' '}
            <a href="https://www.belastingdienst.nl/wps/wcm/connect/nl/btw/content/btw-aangifte-doen-per-maand-kwartaal-of-jaar" target="_blank" rel="noreferrer">{t('bookkeeping.belastingdienst')}</a>
          </p>
        )}
      </div>
    </Block>
  )
}
