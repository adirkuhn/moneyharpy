import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { CaretIcon, SearchIcon } from '../layout/icons'
import { DocumentTable } from '../list/DocumentTable'
import { AdvancedFilter, FilterPanel, RadioFilter, SelectFilter } from '../list/FilterPanel'
import { GettingStarted } from '../list/GettingStarted'
import { ListActions } from '../list/ListActions'
import { useDisclosure } from '../list/useDisclosure'
import { ContactIcon } from './ContactIcon'
import {
  type ContactFilter, contactFilterTerms, contactGroup, contactPerson, parseContactFilter, quickFilterOf, quickFilters,
} from './filter'
import { type Contact, contactName, type ContactSearch, perPage, useContactDoubles, useContacts, useTotalWithArchived } from './queries'
import './contacts.css'

/** The administration's customers and suppliers, after Moneybird's list: searchable, filtered, grouped by letter and paged (/contacts). */
export function ContactsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const filter = parseContactFilter(params.get('filter'))
  const archived = params.get('archived') === '1'
  const duplicates = params.get('duplicates') === '1'
  const page = Math.max(1, Number(params.get('page')) || 1)
  const [query, setQuery] = useState(params.get('q') ?? '')
  const search: ContactSearch = { query: params.get('q') ?? '', filter: contactFilterTerms(filter), archived, page }
  const contacts = useContacts(administrationId, search)
  const withArchived = useTotalWithArchived(administrationId, search, !archived && !duplicates)
  const doubles = useContactDoubles(administrationId)
  const [info, setInfo] = useState<boolean | null>(null)
  const introId = useId()
  const base = `/${administrationId}/contacts`
  const quick = quickFilterOf(filter)
  const doubled = uniqueContacts((doubles.data ?? []).flatMap((group) => group.contacts))
  const listed = duplicates ? (doubles.data ? sortByName(doubled) : undefined) : contacts.data?.contacts
  const total = duplicates ? doubled.length : (contacts.data?.total ?? 0)
  const firstUse = !duplicates && contacts.data !== undefined && listed?.length === 0 && !search.query && quick === 'all' && !archived && page === 1
  const introShown = info ?? firstUse
  const hidden = !archived && contacts.data && withArchived.data !== undefined ? withArchived.data - contacts.data.total : 0

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

  const quickLinks = Object.entries(quickFilters).map(([key, f]) => {
    const terms = contactFilterTerms(f)
    return { label: t(`contacts.quick.${key}`), to: terms ? `${base}?${new URLSearchParams({ filter: terms })}` : base, current: !duplicates && key === quick }
  })
  const filterPanel = (
    <FilterPanel label={duplicates ? t('contacts.views.duplicates') : t(`contacts.views.${quick}`)} title={t('contacts.filters')} quick={quickLinks}>
      <ContactFilterForm administrationId={administrationId} filter={filter} onApply={(f) => set({ filter: contactFilterTerms(f), duplicates: '' })} />
    </FilterPanel>
  )

  return (
    <Page
      wide
      title={t('contacts.title')}
      lead={filterPanel}
      actions={
        <>
          <div className="list-search actionbar__search" role="search" aria-label={t('contacts.search')}>
            <SearchIcon />
            <input type="search" aria-label={t('contacts.search')} placeholder={t('contacts.search')} value={query} onChange={(e) => setQuery(e.target.value)} />
          </div>
          <ListActions
            info={{ shown: introShown, onToggle: () => setInfo(!introShown), controls: introId }}
            exports={[t('contacts.exportExcel'), t('contacts.exportVcard')]}
            trailing={<ImportMenu />}
            primary={{ to: `${base}/new`, label: t('contacts.add') }}
          />
        </>
      }
    >
      {introShown && (
        <GettingStarted
          id={introId}
          title={t('contacts.intro.title')}
          text={t('contacts.intro.text')}
          steps={[
            { icon: 'plus', title: t('contacts.intro.add'), text: t('contacts.intro.addText') },
            { icon: 'flow', title: t('contacts.intro.workflow'), text: t('contacts.intro.workflowText') },
            { icon: 'chart', title: t('contacts.intro.activities'), text: t('contacts.intro.activitiesText') },
          ]}
        />
      )}
      {!duplicates && doubled.length > 0 && <DuplicatesBanner count={doubled.length} to={`${base}?duplicates=1`} />}
      {listed === undefined ? (
        contacts.error || doubles.error ? <p className="empty">{t('contacts.loadError')}</p> : <Loading />
      ) : listed.length === 0 ? (
        <p className="empty">{firstUse ? t('contacts.empty') : t('contacts.noMatches')}</p>
      ) : (
        <ContactsTable administrationId={administrationId} contacts={listed} total={total} />
      )}
      {!duplicates && (archived || hidden > 0) && (
        <p className="contact-list__inactive">
          {!archived && <span>{t('contacts.deactivated', { count: hidden })} </span>}
          <button type="button" className="link-button" onClick={() => set({ archived: archived ? '' : '1' })}>
            {archived ? t('contacts.hideDeactivated') : t('contacts.showDeactivated')}
          </button>
        </p>
      )}
    </Page>
  )
}

/** The contacts under the letter they start with, shown once on the first row of each letter. */
function ContactsTable({ administrationId, contacts, total }: { administrationId: string; contacts: Contact[]; total: number }) {
  const { t } = useTranslation()
  const firsts = new Map<string, string>()
  let previous: string | null = null
  for (const c of contacts) {
    const group = contactGroup(contactName(c))
    if (group !== previous) {
      firsts.set(c.id ?? '', group)
      previous = group
    }
  }
  return (
    <div className="contact-list">
      <DocumentTable
        label={t('contacts.title')}
        rows={contacts}
        rowKey={(c) => c.id ?? ''}
        href={(c) => `/${administrationId}/contacts/${c.id}`}
        total={total}
        perPage={perPage}
        columns={[
          { key: 'letter', header: '', label: t('contacts.letter'), cell: (c) => <span className="contact-list__letter">{firsts.get(c.id ?? '')}</span> },
          {
            key: 'name', header: t('contacts.name'), link: true,
            cell: (c) => {
              const person = contactPerson(c)
              return (
                <>
                  {contactName(c)}
                  {person && <>{' — '}<span className="mute">{person}</span></>}
                  {c.archived && <span className="badge badge--archived">{t('contacts.archived')}</span>}
                </>
              )
            },
          },
        ]}
      />
    </div>
  )
}

/** Moneybird's nudge to merge contacts that look the same. */
function DuplicatesBanner({ count, to }: { count: number; to: string }) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <section className="feature-banner" aria-labelledby={id}>
      <span className="feature-banner__icon"><ContactIcon kind="merge" /></span>
      <div className="feature-banner__text">
        <h2 id={id} className="feature-banner__title">{t('contacts.duplicates', { count })}</h2>
        <p>{t('contacts.duplicatesText')}</p>
      </div>
      <Link className="btn btn--secondary btn--with-icon" to={to}>
        <ContactIcon kind="merge" />
        <span>{t('contacts.viewDuplicates')}</span>
      </Link>
    </section>
  )
}

/** Moneybird's Import button. MoneyHarpy cannot import contact files yet, so its panel says so. */
function ImportMenu() {
  const { t } = useTranslation()
  const id = useId()
  const { open, toggle, root, button } = useDisclosure()
  return (
    <div className="dropdown" ref={root}>
      <button ref={button} type="button" className="btn btn--secondary btn--with-icon" aria-expanded={open} aria-controls={id} onClick={toggle}>
        <ContactIcon kind="import" />
        <span>{t('contacts.import')}</span>
        <CaretIcon />
      </button>
      {open && (
        <div id={id} className="dropdown__panel dropdown__panel--right contact-import">
          <p>{t('contacts.importText')}</p>
          <p className="hint">{t('list.notAvailable')}</p>
        </div>
      )}
    </div>
  )
}

/** The advanced filter of the contacts, with Moneybird's fields in Moneybird's order. */
function ContactFilterForm({ administrationId, filter, onApply }: { administrationId: string; filter: ContactFilter; onApply: (filter: ContactFilter) => void }) {
  const { t } = useTranslation()
  const workflows = useWorkflows(administrationId)
  const [chosen, setChosen] = useState<ContactFilter>(filter)
  const all = { value: '', label: t('list.all') }
  const pick = <K extends keyof ContactFilter>(key: K) => (value: string) => setChosen({ ...chosen, [key]: value as ContactFilter[K] })
  const flows = (type: string) => [
    { value: '', label: t('list.allWorkflows') },
    ...(workflows.data ?? []).filter((w) => w.type === type).map((w) => ({ value: w.id ?? '', label: w.name ?? '' })),
  ]
  return (
    <AdvancedFilter value={chosen} onApply={() => onApply(chosen)}>
      <div className="filter-columns">
        <RadioFilter legend={t('contacts.filter.contactType')} value={chosen.contact_type} onChange={pick('contact_type')}
          options={[all, { value: 'company', label: t('contacts.filter.company') }, { value: 'private_individual', label: t('contacts.filter.private') }]} />
        <RadioFilter legend={t('contacts.filter.trusted')} value={chosen.trusted_type} onChange={pick('trusted_type')}
          options={[all, { value: 'trusted', label: t('contacts.filter.isTrusted') }, { value: 'not_trusted', label: t('contacts.filter.notTrusted') }]} />
        <SelectFilter label={t('contacts.filter.estimateWorkflow')} value={chosen.estimate_workflow_id} onChange={pick('estimate_workflow_id')} options={flows('EstimateWorkflow')} />
        <SelectFilter label={t('contacts.filter.invoiceWorkflow')} value={chosen.invoice_workflow_id} onChange={pick('invoice_workflow_id')} options={flows('InvoiceWorkflow')} />
      </div>
      <RadioFilter legend={t('contacts.filter.mandate')} value={chosen.mandate_type} onChange={pick('mandate_type')}
        options={[all, { value: 'manual', label: t('contacts.filter.manualMandate') }, { value: 'adyen', label: t('contacts.filter.moneybirdMandate') }, { value: 'none', label: t('contacts.filter.noMandate') }]} />
      <RadioFilter legend={t('contacts.filter.deliveryMethod')} value={chosen.delivery_method} onChange={pick('delivery_method')}
        options={[all, { value: 'email', label: t('contacts.filter.email') }, { value: 'peppol', label: t('contacts.filter.peppol') }, { value: 'manual', label: t('contacts.filter.manual') }]} />
    </AdvancedFilter>
  )
}

function uniqueContacts(list: Contact[]) {
  const seen = new Set<string>()
  return list.filter((c) => !seen.has(c.id ?? '') && seen.add(c.id ?? ''))
}

function sortByName(list: Contact[]) {
  return [...list].sort((a, b) => contactName(a).localeCompare(contactName(b), undefined, { sensitivity: 'base' }))
}
