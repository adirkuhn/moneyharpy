import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Pagination } from '../layout/Pagination'
import { SearchIcon } from '../layout/icons'
import { contactName, type ContactSearch, perPage, useContacts } from './queries'

const kinds = ['', 'company', 'private_individual'] as const

/** The administration's customers and suppliers, searchable and paged. */
export function ContactsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const kind = (kinds as readonly string[]).includes(params.get('kind') ?? '') ? (params.get('kind') as ContactSearch['kind']) : ''
  const archived = params.get('archived') === '1'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const [query, setQuery] = useState(params.get('q') ?? '')
  const search: ContactSearch = { query: params.get('q') ?? '', kind, archived, page }
  const contacts = useContacts(administrationId, search)

  const set = (changes: Record<string, string>) => {
    const next = new URLSearchParams(params)
    for (const [key, value] of Object.entries(changes)) {
      if (value) {
        next.set(key, value)
      } else {
        next.delete(key)
      }
    }
    next.delete('page')
    setParams(next, { replace: true })
  }

  // Searching waits for a pause in typing, so each key does not ask the server.
  useEffect(() => {
    if (query === (params.get('q') ?? '')) {
      return
    }
    const timer = setTimeout(() => set({ q: query.trim() }), 250)
    return () => clearTimeout(timer)
  })

  return (
    <Page title={t('contacts.title')} actions={<Link className="btn btn--accent" to={`/${administrationId}/contacts/new`}>{t('contacts.new')}</Link>}>
      <div className="filter-bar" role="search" aria-label={t('contacts.filters')}>
        <label className="filter-bar__field filter-bar__search">
          <span className="sr-only">{t('contacts.search')}</span>
          <SearchIcon />
          <input type="search" aria-label={t('contacts.search')} placeholder={t('contacts.search')} value={query} onChange={(e) => setQuery(e.target.value)} />
        </label>
        <label className="filter-bar__field">
          <span>{t('contacts.kind')}</span>
          <select value={kind} onChange={(e) => set({ kind: e.target.value })}>
            {kinds.map((k) => <option key={k} value={k}>{t(`contacts.kinds.${k || 'all'}`)}</option>)}
          </select>
        </label>
        <label className="filter-bar__check">
          <input type="checkbox" checked={archived} onChange={(e) => set({ archived: e.target.checked ? '1' : '' })} />
          {t('contacts.showArchived')}
        </label>
      </div>
      {!contacts.data ? (
        contacts.error ? <p className="empty">{t('contacts.loadError')}</p> : <Loading />
      ) : contacts.data.contacts.length === 0 ? (
        <p className="empty">{search.query || kind ? t('contacts.noMatches') : t('contacts.empty')}</p>
      ) : (
        <>
          <table className="data-table contacts" aria-label={t('contacts.title')}>
            <thead>
              <tr>
                <th scope="col" className="contacts__number"><abbr title={t('contacts.customerId')}>{t('contacts.numberShort')}</abbr></th>
                <th scope="col">{t('contacts.name')}</th>
                <th scope="col" className="data-table__extra">{t('contacts.email')}</th>
                <th scope="col" className="data-table__extra">{t('contacts.city')}</th>
              </tr>
            </thead>
            <tbody>
              {contacts.data.contacts.map((c) => (
                <tr key={c.id} className={c.archived ? 'is-inactive' : undefined}>
                  <td className="contacts__number">{c.customer_id}</td>
                  <td>
                    <Link to={`/${administrationId}/contacts/${c.id}`}>{contactName(c)}</Link>
                    {c.archived && <span className="ledger__state">{t('contacts.archived')}</span>}
                  </td>
                  <td className="data-table__extra">{c.email}</td>
                  <td className="data-table__extra">{c.city}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pagination total={contacts.data.total} perPage={perPage} />
        </>
      )}
    </Page>
  )
}
