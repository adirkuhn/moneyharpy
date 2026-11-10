import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { formErrors, TextInput } from '../forms/Form'
import { Block } from '../settings/Block'
import { type BookkeepingSettings, useUpdateBookkeepingSettings } from './queries'

/** Closes the books up to a date, so nothing before it changes by accident. */
export function LockBlock({ administrationId, settings }: { administrationId: string; settings: BookkeepingSettings }) {
  const { t, i18n } = useTranslation()
  const update = useUpdateBookkeepingSettings(administrationId)
  const [day, setDay] = useState(settings.period_locked_until ?? '')
  const errors = formErrors(update.error, { period_locked_until: t('bookkeeping.lockDate') })
  const today = new Date().toISOString().slice(0, 10)
  const locked = settings.period_locked_until
  const submit = (e: FormEvent) => {
    e.preventDefault()
    update.mutate({ period_locked_until: day })
  }
  return (
    <Block id="locked-period" title={t('bookkeeping.parts.lock')} subtitle={t('bookkeeping.lockSubtitle')}>
      <form className="card card-stack" onSubmit={submit} noValidate>
        <p className="card__title">
          {locked ? t('bookkeeping.lockedUntil', { date: new Date(`${locked}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'long' }) }) : t('bookkeeping.notLocked')}
        </p>
        <Problems messages={errors.general} />
        <TextInput label={t('bookkeeping.lockDate')} type="date" width="medium" max={today} value={day} errors={errors.field('period_locked_until')} onChange={(e) => setDay(e.target.value)} />
        <div className="confirm-row__buttons">
          <button type="submit" className="btn btn--accent btn--small" disabled={update.isPending || !day || day === locked}>{t('bookkeeping.lock')}</button>
          {locked && (
            <button type="button" className="btn btn--secondary btn--small" disabled={update.isPending} onClick={() => update.mutate({ period_locked_until: '' }, { onSuccess: () => setDay('') })}>
              {t('bookkeeping.unlock')}
            </button>
          )}
        </div>
      </form>
    </Block>
  )
}
