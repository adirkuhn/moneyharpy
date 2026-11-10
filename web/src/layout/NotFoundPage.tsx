import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Page } from './Page'

export function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <Page title={t('notFound.title')}>
      <p className="empty">
        {t('notFound.body')} <Link to="/administrations">{t('notFound.back')}</Link>
      </p>
    </Page>
  )
}
