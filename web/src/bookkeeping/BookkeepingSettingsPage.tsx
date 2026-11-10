import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Block } from '../settings/Block'
import { CategoriesBlock } from './CategoriesBlock'
import { LockBlock } from './LockBlock'
import { PeriodsBlock } from './PeriodsBlock'
import { useBookkeepingSettings } from './queries'
import { SchemeBlock } from './SchemeBlock'
import { TaxRatesBlock } from './TaxRatesBlock'

/** Settings > Bookkeeping, block by block like Moneybird. */
export function BookkeepingSettingsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const settings = useBookkeepingSettings(administrationId)
  const { hash } = useLocation()
  const ready = Boolean(settings.data && administration.data)

  useEffect(() => {
    if (hash && ready) {
      document.getElementById(hash.slice(1))?.scrollIntoView?.({ block: 'start' })
    }
  }, [hash, ready])

  if (!settings.data || !administration.data) {
    return <Loading />
  }
  const s = settings.data
  return (
    <Page title={t('bookkeeping.title')} subtitle={t('bookkeeping.subtitle')}>
      <PeriodsBlock administrationId={administrationId} settings={s} country={administration.data.country} />
      <TaxRatesBlock administrationId={administrationId} />
      <CategoriesBlock administrationId={administrationId} hasScheme={s.chart_scheme !== ''} />
      <SchemeBlock key={s.chart_scheme} administrationId={administrationId} settings={s} />
      <LockBlock key={s.period_locked_until ?? ''} administrationId={administrationId} settings={s} />
      <Block id="export" title={t('bookkeeping.parts.export')} subtitle={t('bookkeeping.exportSubtitle')}>
        <p className="empty">{t('bookkeeping.exportUnavailable')}</p>
      </Block>
    </Page>
  )
}
