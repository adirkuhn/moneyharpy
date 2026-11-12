import { type ReactNode, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { useContactDocuments } from '../contacts/documents'
import { type Contact, contactName, useContact } from '../contacts/queries'
import { addressLines } from './paper'

type Shown = { id?: string | null; created_at?: string | null; email?: string | null; send_invoices_to_email?: string | null }
  & Pick<Contact, 'company_name' | 'firstname' | 'lastname' | 'address1' | 'address2' | 'zipcode' | 'city'>
type Props = { administrationId: string; contact: Shown }

/** "about 22 hours ago": the time since a moment in the largest unit that fits. */
export function timeAgo(iso: string, language: string, now = new Date()) {
  const seconds = (new Date(iso).getTime() - now.getTime()) / 1000
  const units: [Intl.RelativeTimeFormatUnit, number][] = [['year', 31536000], ['month', 2592000], ['day', 86400], ['hour', 3600], ['minute', 60]]
  const rtf = new Intl.RelativeTimeFormat(language, { numeric: 'auto' })
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size) {
      return rtf.format(Math.round(seconds / size), unit)
    }
  }
  return rtf.format(0, 'minute')
}

/**
 * A link to a contact that shows a card about it while it is hovered or
 * focused, after Moneybird's hover cards: its name, when it was created, its
 * address and email, and how many invoices and documents it has.
 */
export function ContactHoverCard({ administrationId, contact }: Props) {
  return (
    <HoverCard administrationId={administrationId} contact={contact}>
      {(describedBy) => <Link to={`/${administrationId}/contacts/${contact.id}`} aria-describedby={describedBy}>{contactName(contact)}</Link>}
    </HoverCard>
  )
}

/**
 * Shows the card of a contact while what it wraps is hovered or focused,
 * such as the chosen contact of the invoice popup's selector.
 */
export function HoverCard({ administrationId, contact, className, children }: Props & { className?: string; children: (describedBy: string | undefined) => ReactNode }) {
  const [open, setOpen] = useState(false)
  const id = useId()
  const timer = useRef<number | undefined>(undefined)
  const show = () => {
    window.clearTimeout(timer.current)
    setOpen(true)
  }
  // A short delay lets the pointer move from the link onto the card.
  const hide = () => {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setOpen(false), 150)
  }
  const to = `/${administrationId}/contacts/${contact.id}`
  return (
    <span
      className={`hover-card${className ? ` ${className}` : ''}`}
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocus={show}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          hide()
        }
      }}
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          setOpen(false)
        }
      }}
    >
      {children(open ? id : undefined)}
      {open && <Card id={id} administrationId={administrationId} contact={contact} to={to} />}
    </span>
  )
}

function Card({ id, administrationId, contact: known, to }: Props & { id: string; to: string }) {
  const { t, i18n } = useTranslation()
  const full = useContact(administrationId, known.id ?? undefined)
  const { documents } = useContactDocuments(administrationId, known.id ?? '')
  const c = full.data ?? known
  const name = contactName(c)
  const address = addressLines(c)
  const email = c.email || c.send_invoices_to_email
  const count = (kinds: string[]) => documents.filter((d) => kinds.includes(d.kind)).reduce((n, d) => n + d.total, 0)
  return (
    <span id={id} className="hover-card__content" role="dialog" aria-label={name}>
      <span className="hover-card__header">
        <Link className="hover-card__name" to={to}>{name}</Link>
        {c.created_at && <span className="hover-card__meta">{t('invoices.view.card.created', { ago: timeAgo(c.created_at, i18n.language) })}</span>}
      </span>
      {(address.length > 0 || email) && (
        <span className="hover-card__grid">
          {address.length > 0 && (
            <>
              <span className="hover-card__label">{t('invoices.view.card.address')}</span>
              <span className="hover-card__value">{address.map((l, i) => <span key={i} className="hover-card__line">{l}</span>)}</span>
            </>
          )}
          {email && (
            <>
              <span className="hover-card__label">{t('invoices.view.card.email')}</span>
              <span className="hover-card__value">{email}</span>
            </>
          )}
        </span>
      )}
      <span className="hover-card__stats">
        <span className="hover-card__stat"><strong>{count(['invoice'])}</strong> <span>{t('invoices.view.card.invoices')}</span></span>
        <span className="hover-card__stat"><strong>{count(['purchase', 'receipt'])}</strong> <span>{t('invoices.view.card.documents')}</span></span>
      </span>
    </span>
  )
}
