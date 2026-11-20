import { useTranslation } from 'react-i18next'
import { Page } from './Page'

/** Shown on a page the member has no permission for. */
export function NoAccess() {
  const { t } = useTranslation()
  return (
    <Page title={t('management.noAccessTitle')}>
      <p className="empty">{t('management.noAccess')}</p>
    </Page>
  )
}
