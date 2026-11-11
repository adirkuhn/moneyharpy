import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { SearchIcon } from '../layout/icons'
import { DocumentTable } from '../list/DocumentTable'
import { AdvancedFilter, CheckFilter, FilterPanel, RadioFilter } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import { contactName, type ContactSearch, perPage, useContacts } from './queries'

const kinds = ['', 'company', 'private_individual'] as const
type Kind = ContactSearch['kind']

/** The quick filters of the contacts, as the address they lead to. */
const quickFilters: Record<string, { kind: Kind; archived: boolean }> = {
  all: { kind: '', archived: false },
  company: { kind: 'company', archived: false },
  private_individual: { kind: 'private_individual', archived: false },
  archived: { kind: '', archived: true },
}

/** The administration's customers and suppliers, searchable, filtered by kind and paged (/contacts). */
export function ContactsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const kind = (kinds as readonly string[]).includes(params.get('kind') ?? '') ? (params.get('kind') as Kind) : ''
  const archived = params.get('archived') === '1'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const [query, setQuery] = useState(params.get('q') ?? '')
  const search: ContactSearch = { query: params.get('q') ?? '', kind, archived, page }
  const contacts = useContacts(administrationId, search)
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const base = `/${administrationId}/contacts`
  const shown = contacts.data?.contacts ?? []
  const firstUse = contacts.data !== undefined && shown.length === 0 && !search.query && !kind && !archived && page === 1
  const introShown = info ?? firstUse

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

  const view = t(`contacts.kinds.${kind || 'all'}`)
  const quick = Object.entries(quickFilters).map(([key, f]) => {
    const to = new URLSearchParams({ ...(f.kind ? { kind: f.kind } : {}), ...(f.archived ? { archived: '1' } : {}) }).toString()
    return { label: t(`contacts.quick.${key}`), to: to ? `${base}?${to}` : base, current: f.kind === kind && f.archived === archived }
  })
  const filterPanel = (
    <FilterPanel label={archived ? t('contacts.viewWithArchived', { view }) : view} title={t('contacts.filters')} quick={quick}>
      <ContactFilter kind={kind} archived={archived} onApply={(k, a) => set({ kind: k, archived: a ? '1' : '' })} />
    </FilterPanel>
  )

  return (
    <Page
      wide
      title={t('contacts.title')}
      lead={filterPanel}
      actions={
        <ListActions
          info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
          exports={[]}
          primary={{ to: `${base}/new`, label: t('contacts.new') }}
        />
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('contacts.intro.title')}
          text={t('contacts.intro.text')}
          steps={[
            { icon: 'plus', title: t('contacts.intro.add'), text: t('contacts.intro.addText') },
            { icon: 'at', title: t('contacts.intro.people'), text: t('contacts.intro.peopleText') },
            { icon: 'file', title: t('contacts.intro.invoice'), text: t('contacts.intro.invoiceText') },
          ]}
        />
      )}
      <div className="list-search" role="search" aria-label={t('contacts.search')}>
        <SearchIcon />
        <input type="search" aria-label={t('contacts.search')} placeholder={t('contacts.search')} value={query} onChange={(e) => setQuery(e.target.value)} />
      </div>
      {!contacts.data ? (
        contacts.error ? <p className="empty">{t('contacts.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{search.query || kind ? t('contacts.noMatches') : t('contacts.empty')}</p>
      ) : (
        <DocumentTable
          label={t('contacts.title')}
          rows={shown}
          rowKey={(c) => c.id ?? ''}
          href={(c) => `${base}/${c.id}`}
          total={contacts.data.total}
          perPage={perPage}
          columns={[
            {
              key: 'name', header: t('contacts.name'), link: true,
              cell: (c) => (
                <>
                  {contactName(c)}
                  {c.archived && <span className="badge badge--archived">{t('contacts.archived')}</span>}
                </>
              ),
            },
            { key: 'number', header: t('contacts.customerId'), cell: (c) => c.customer_id },
            { key: 'email', header: t('contacts.email'), kind: 'extra', cell: (c) => c.email || c.send_invoices_to_email },
            { key: 'city', header: t('contacts.city'), kind: 'extra', cell: (c) => c.city },
          ]}
        />
      )}
    </Page>
  )
}

/** The advanced filter of the contacts: their kind and whether archived ones show. */
function ContactFilter({ kind, archived, onApply }: { kind: Kind; archived: boolean; onApply: (kind: Kind, archived: boolean) => void }) {
  const { t } = useTranslation()
  const [chosen, setChosen] = useState<Kind>(kind)
  const [withArchived, setWithArchived] = useState(archived)
  return (
    <AdvancedFilter value={{ chosen, withArchived }} onApply={() => onApply(chosen, withArchived)}>
      <RadioFilter legend={t('contacts.kind')} value={chosen} onChange={(v) => setChosen(v as Kind)} options={kinds.map((k) => ({ value: k, label: t(`contacts.kinds.${k || 'all'}`) }))} />
      <CheckFilter label={t('contacts.showArchived')} checked={withArchived} onChange={setWithArchived} />
    </AdvancedFilter>
  )
}
