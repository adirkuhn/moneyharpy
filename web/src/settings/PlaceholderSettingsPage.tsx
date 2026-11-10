import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type Section, sections } from './SettingsLayout'

/** A settings section whose feature is not built yet. */
export function PlaceholderSettingsPage() {
  const { t } = useTranslation()
  const { section = '' } = useParams()
  if (!sections.includes(section as Section)) {
    return <NotFoundPage />
  }
  return (
    <Page title={t(`settings.sections.${section}`)}>
      <p className="empty">{t('settings.notAvailable')}</p>
    </Page>
  )
}
