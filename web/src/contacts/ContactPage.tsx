import { type KeyboardEvent, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { ConfirmPanel } from '../detail/Detail'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { ActionMenu } from '../list/ListActions'
import { ContactCard } from './ContactCard'
import { ContactDetails } from './ContactDetails'
import { ContactFeed } from './ContactFeed'
import { ContactIcon } from './ContactIcon'
import { ContactNotes } from './ContactNotes'
import { useContactDocuments } from './documents'
import { type Contact, contactName, useArchiveContact, useContact, useDeleteContact, useSaveContact } from './queries'
import './contacts.css'

const tabs = ['feed', 'details', 'notes'] as const
type Tab = (typeof tabs)[number]

/**
 * One contact, after Moneybird's contact page: a card with the contact's
 * name, ways to reach it, its documents and people, beside tabs with its
 * feed, its details and its notes (/contacts/:id).
 */
export function ContactPage() {
  const { administrationId = '', contactId = '' } = useParams()
  const contact = useContact(administrationId, contactId)
  if (contact.error instanceof ApiError && contact.error.status < 500) {
    return <NotFoundPage />
  }
  if (!contact.data) {
    return <Loading />
  }
  return <ContactView administrationId={administrationId} contact={contact.data} />
}

function ContactView({ administrationId, contact: c }: { administrationId: string; contact: Contact }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const id = c.id ?? ''
  const archive = useArchiveContact(administrationId, id)
  const restore = useSaveContact(administrationId, id)
  const remove = useDeleteContact(administrationId, id)
  const documents = useContactDocuments(administrationId, id)
  const [confirming, setConfirming] = useState<'archive' | 'delete' | null>(null)
  const list = `/${administrationId}/contacts`
  const name = contactName(c)
  const problems = formErrors(archive.error ?? restore.error ?? remove.error, {}).general
  const withContact = `contact_id=${id}`

  const actions = (
    <>
      <Link className="btn btn--secondary btn--with-icon" to={`${list}/${id}/edit`}>
        <ContactIcon kind="pen" />
        <span>{t('contacts.edit')}</span>
      </Link>
      <ActionMenu
        label={t('contacts.view.export')}
        icon={<ListIcon kind="export" />}
        items={[{ label: t('contacts.exportExcel'), unavailable: true }, { label: t('contacts.exportVcard'), unavailable: true }]}
      />
      <ActionMenu
        label={t('contacts.view.extra')}
        items={[
          { label: t('contacts.view.merge'), unavailable: true },
          { label: t('contacts.view.mandate'), to: `${list}/${id}/edit` },
          { label: t('contacts.view.events'), unavailable: true },
          { label: t('contacts.view.delete'), danger: true, onSelect: () => setConfirming('delete') },
          c.archived
            ? { label: t('contacts.view.unarchive'), disabled: restore.isPending, onSelect: () => restore.mutate({ archived: false }) }
            : { label: t('contacts.view.archive'), onSelect: () => setConfirming('archive') },
        ]}
      />
      <ActionMenu
        primary
        label={t('contacts.view.add')}
        icon={<ListIcon kind="plus" />}
        items={[
          { label: t('contacts.view.adds.invoice'), to: `/${administrationId}/sales_invoices/new?${withContact}` },
          { label: t('contacts.view.adds.external'), to: `/${administrationId}/external_sales_invoices/new?${withContact}` },
          { label: t('contacts.view.adds.recurring'), to: `/${administrationId}/recurring_sales_invoices/new?${withContact}` },
          { label: t('contacts.view.adds.subscription'), unavailable: true },
          { label: t('contacts.view.adds.purchase'), unavailable: true },
          { label: t('contacts.view.adds.receipt'), unavailable: true },
          { label: t('contacts.view.adds.general'), unavailable: true },
          { label: t('contacts.view.adds.quote'), to: `/${administrationId}/estimates/new?${withContact}` },
        ]}
      />
    </>
  )

  return (
    <Page wide title={t('contacts.view.title')} actions={actions}>
      <Problems messages={problems} />
      {confirming === 'delete' && (
        <ConfirmPanel
          message={t('contacts.view.confirmDelete', { name })}
          description={t('contacts.view.confirmDeleteText')}
          confirmLabel={t('contacts.view.deleteOk')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
          onCancel={() => setConfirming(null)}
        />
      )}
      {confirming === 'archive' && (
        <ConfirmPanel
          danger={false}
          message={t('contacts.view.confirmArchive', { name })}
          description={t('contacts.view.confirmArchiveText')}
          confirmLabel={t('contacts.view.archiveOk')}
          disabled={archive.isPending}
          onConfirm={() => archive.mutate(undefined, { onSuccess: () => setConfirming(null) })}
          onCancel={() => setConfirming(null)}
        />
      )}
      {c.archived && <p className="notice">{t('contacts.archivedNotice')}</p>}
      <div className="contact-view">
        <ContactCard administrationId={administrationId} contact={c} documents={documents.documents} />
        <ContactTabs administrationId={administrationId} contact={c} documents={documents} />
      </div>
    </Page>
  )
}

type TabsProps = { administrationId: string; contact: Contact; documents: ReturnType<typeof useContactDocuments> }

/** Feed, Details and Tasks and notes, as Moneybird's tab bar; the chosen tab is kept in the address. */
function ContactTabs({ administrationId, contact, documents }: TabsProps) {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const chosen = params.get('tab')
  const current: Tab = tabs.find((tab) => tab === chosen) ?? 'feed'
  const base = useId()
  const buttons = useRef<Record<string, HTMLButtonElement | null>>({})

  const select = (tab: Tab) => {
    const next = new URLSearchParams(params)
    if (tab === 'feed') {
      next.delete('tab')
    } else {
      next.set('tab', tab)
    }
    setParams(next, { replace: true })
  }
  // Arrow keys move along the tabs and choose them, as in any tab bar.
  const onKeyDown = (e: KeyboardEvent) => {
    const step = { ArrowRight: 1, ArrowLeft: -1 }[e.key]
    const edge = { Home: 0, End: tabs.length - 1 }[e.key]
    if (step === undefined && edge === undefined) {
      return
    }
    e.preventDefault()
    const index = edge ?? (tabs.indexOf(current) + (step ?? 0) + tabs.length) % tabs.length
    const tab = tabs[index] ?? 'feed'
    select(tab)
    buttons.current[tab]?.focus()
  }

  return (
    <div className="contact-view__main">
      <div className="contact-tabs" role="tablist" aria-label={t('contacts.view.sections')} onKeyDown={onKeyDown}>
        {tabs.map((tab) => (
          <button
            key={tab}
            ref={(el) => { buttons.current[tab] = el }}
            type="button"
            role="tab"
            id={`${base}-${tab}`}
            className="contact-tabs__tab"
            aria-selected={tab === current}
            aria-controls={`${base}-${tab}-panel`}
            tabIndex={tab === current ? 0 : -1}
            onClick={() => select(tab)}
          >
            {t(`contacts.view.tabs.${tab}`)}
          </button>
        ))}
      </div>
      <div role="tabpanel" id={`${base}-${current}-panel`} aria-labelledby={`${base}-${current}`} className="contact-view__panel">
        {current === 'feed' && <ContactFeed administrationId={administrationId} contact={contact} documents={documents.documents} pending={documents.pending} />}
        {current === 'details' && <ContactDetails administrationId={administrationId} contact={contact} />}
        {current === 'notes' && <ContactNotes administrationId={administrationId} contact={contact} />}
      </div>
    </div>
  )
}
