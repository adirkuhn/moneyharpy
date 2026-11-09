import { Link } from 'react-router-dom'
import type { Administration } from '../api/client'
import { ChevronIcon, CogIcon, StarIcon } from '../layout/icons'
import { Page } from '../layout/Page'
import { useAdministrations, useSetFavorite } from './queries'

export function AdministrationsPage() {
  const administrations = useAdministrations()
  return (
    <Page
      title="Select an administration"
      actions={<Link className="btn btn--accent" to="/administrations/new">Add administration</Link>}
    >
      {administrations.isError && (
        <p role="alert" className="problems">Administrations could not be loaded. Reload the page to try again.</p>
      )}
      {administrations.data?.length === 0 && (
        <p className="empty">Add your first administration to start bookkeeping.</p>
      )}
      {administrations.data && administrations.data.length > 0 && (
        <ul className="ledger" aria-label="Administrations">
          {administrations.data.map((a) => <Row key={a.id} administration={a} />)}
        </ul>
      )}
    </Page>
  )
}

function Row({ administration: a }: { administration: Administration }) {
  const setFavorite = useSetFavorite()
  return (
    <li className="ledger__row">
      <button
        type="button"
        className={`icon-btn star${a.favorite ? ' star--on' : ''}`}
        aria-pressed={a.favorite}
        aria-label={a.favorite ? `Remove ${a.name} from favorites` : `Mark ${a.name} as favorite`}
        onClick={() => setFavorite.mutate({ id: a.id, favorite: !a.favorite })}
      >
        <StarIcon filled={a.favorite} />
      </button>
      <Link className="ledger__name" to={a.uri}>{a.name}</Link>
      <Link className="icon-btn" to={`${a.uri}/settings`} aria-label={`Settings for ${a.name}`}><CogIcon /></Link>
      <ChevronIcon className="ledger__chevron" />
    </li>
  )
}
