import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { errorMessages } from '../api/client'
import { SelectInput } from '../forms/Form'
import { Block } from '../settings/Block'
import { type BookkeepingSettings, useUpdateBookkeepingSettings } from './queries'

/** Which standard chart of accounts the categories map to, such as RGS in the Netherlands. */
export function SchemeBlock({ administrationId, settings }: { administrationId: string; settings: BookkeepingSettings }) {
  const { t } = useTranslation()
  const update = useUpdateBookkeepingSettings(administrationId)
  const [choice, setChoice] = useState(settings.chart_scheme)
  const changed = choice !== settings.chart_scheme
  return (
    <Block id="chart-scheme" title={t('bookkeeping.parts.scheme')} subtitle={t('bookkeeping.schemeSubtitle')}>
      <div className="card card-stack">
        <SelectInput label={t('bookkeeping.scheme')} width="long" value={choice} disabled={update.isPending} onChange={(e) => setChoice(e.target.value)}>
          <option value="">{t('bookkeeping.noScheme')}</option>
          {settings.chart_schemes.map((s) => (
            <option key={s.id} value={s.id}>{`${s.name} ${s.version} (${s.country})`}</option>
          ))}
        </SelectInput>
        {changed && (
          <div className="confirm-row" role="alert">
            <p>{settings.chart_scheme ? t('bookkeeping.confirmScheme') : t('bookkeeping.confirmNewScheme')}</p>
            <div className="confirm-row__buttons">
              <button type="button" className="btn btn--accent btn--small" disabled={update.isPending} onClick={() => update.mutate({ chart_scheme: choice })}>{t('bookkeeping.change')}</button>
              <button type="button" className="btn btn--secondary btn--small" onClick={() => setChoice(settings.chart_scheme)}>{t('common.cancel')}</button>
            </div>
          </div>
        )}
        <Problems messages={errorMessages(update.error, {})} />
      </div>
    </Block>
  )
}
