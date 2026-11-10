import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import type { Administration } from '../api/client'
import { ChevronIcon, CogIcon, StarIcon } from '../layout/icons'
import { Page } from '../layout/Page'
import { createPath } from '../onboarding/queries'
import { useAdministrations, useSetFavorite } from './queries'

export function AdministrationsPage() {
  const { t } = useTranslation()
  const administrations = useAdministrations()
  return (
    <Page
      title={t('administrations.title')}
      actions={<Link className="btn btn--accent" to={createPath}>{t('administrations.add')}</Link>}
    >
      {administrations.isError && (
        <p role="alert" className="problems">{t('administrations.loadError')}</p>
      )}
      {administrations.data?.length === 0 && (
        <p className="empty">{t('administrations.empty')}</p>
      )}
      {administrations.data && administrations.data.length > 0 && (
        <ul className="ledger" aria-label={t('administrations.list')}>
          {administrations.data.map((a) => <Row key={a.id} administration={a} />)}
        </ul>
      )}
    </Page>
  )
}

function Row({ administration: a }: { administration: Administration }) {
  const { t } = useTranslation()
  const setFavorite = useSetFavorite()
  return (
    <li className="ledger__row">
      <button
        type="button"
        className={`icon-btn star${a.favorite ? ' star--on' : ''}`}
        aria-pressed={a.favorite}
        aria-label={t(a.favorite ? 'administrations.unfavorite' : 'administrations.favorite', { name: a.name })}
        onClick={() => setFavorite.mutate({ id: a.id, favorite: !a.favorite })}
      >
        <StarIcon filled={a.favorite} />
      </button>
      <Link className="ledger__name" to={a.uri}>{a.name}</Link>
      <Link className="icon-btn" to={`${a.uri}/settings`} aria-label={t('administrations.settingsFor', { name: a.name })}><CogIcon /></Link>
      <ChevronIcon className="ledger__chevron" />
    </li>
  )
}
