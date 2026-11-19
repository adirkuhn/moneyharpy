import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

/**
 * Moneybird's footer under a list that hides its deactivated rows: how many
 * there are, with a link that shows them, or hides them again once shown.
 */
export function Deactivated({ count, shown, show, hide }: { count: number; shown: boolean; show: string; hide: string }) {
  const { t } = useTranslation()
  return (
    <span className="list-deactivated">
      {t('list.deactivated', { count })}{' '}
      {shown ? <Link to={hide}>{t('list.hideDeactivated')}</Link> : count > 0 && <Link to={show}>{t('list.showDeactivated')}</Link>}
    </span>
  )
}
