import { useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { type CSSProperties, type ReactNode, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api, ApiError } from '../api/client'
import { type Contact, contactName, useContact } from './queries'
import './hovercard.css'

/** How long the pointer rests on a name before its card shows, and how long the card stays once it leaves, after Moneybird. */
export const hoverDelays = { showMs: 200, hideMs: 100 }

/**
 * Rails' distance in words, as Moneybird says how long ago a contact was
 * made: "less than a minute", "about 2 hours", "over 2 years".
 */
export function distanceInWords(iso: string, t: TFunction, now = new Date()) {
  const minutes = Math.round(Math.abs(now.getTime() - new Date(iso).getTime()) / 60000)
  const w = (key: string, count?: number) => t(`contacts.hoverCard.distance.${key}`, { count })
  // Below each bound in minutes, the words and their count.
  const steps: [number, string, number][] = [
    [1, 'lessThanMinute', 0],
    [45, 'minutes', minutes],
    [90, 'aboutHours', 1],
    [1440, 'aboutHours', Math.round(minutes / 60)],
    [2520, 'days', 1],
    [43200, 'days', Math.round(minutes / 1440)],
    [86400, 'aboutMonths', Math.round(minutes / 43200)],
    [525600, 'months', Math.round(minutes / 43200)],
  ]
  const step = steps.find(([bound]) => minutes < bound)
  if (step) {
    return w(step[1], step[2])
  }
  const years = Math.floor(minutes / 525600)
  const rest = minutes % 525600
  if (rest < 131400) {
    return w('aboutYears', years)
  }
  return rest < 394200 ? w('overYears', years) : w('almostYears', years + 1)
}

type Named = { id?: string | number | null } & Pick<Contact, 'company_name' | 'firstname' | 'lastname'>

/** A link to a contact that shows its card while it is hovered or focused. */
export function ContactHoverCard({ administrationId, contact }: { administrationId: string; contact: Named }) {
  const name = contactName(contact)
  const id = String(contact.id ?? '')
  return (
    <HoverCard administrationId={administrationId} contactId={id} name={name}>
      {(describedBy) => <Link to={`/${administrationId}/contacts/${id}`} aria-describedby={describedBy}>{name}</Link>}
    </HoverCard>
  )
}

/** A contact's name as text, such as in a row that is itself a link, which shows its card while it is hovered. */
export function ContactName({ administrationId, contactId, name }: { administrationId: string; contactId?: string | null; name: string }) {
  if (!contactId) {
    return <>{name}</>
  }
  return (
    <HoverCard administrationId={administrationId} contactId={contactId} name={name}>
      {(describedBy) => <span aria-describedby={describedBy}>{name}</span>}
    </HoverCard>
  )
}

type HoverCardProps = {
  administrationId: string
  contactId: string
  /** The name the card shows until the contact is loaded. */
  name?: string
  className?: string
  /** What shows the card: a link or button keeps the keyboard's focus, which shows it too. */
  children: (describedBy: string | undefined) => ReactNode
}

/**
 * Shows the card of a contact while what it wraps is hovered or focused, after
 * Moneybird: a moment after the pointer rests on it, above it where there is
 * room, and as long as the pointer is on the name or the card. Escape hides it.
 */
export function HoverCard({ administrationId, contactId, name, className, children }: HoverCardProps) {
  const [open, setOpen] = useState(false)
  const id = useId()
  const anchor = useRef<HTMLSpanElement>(null)
  const timer = useRef<number | undefined>(undefined)
  const later = (fn: () => void, ms: number) => {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(fn, ms)
  }
  const show = () => later(() => setOpen(true), hoverDelays.showMs)
  const stay = () => window.clearTimeout(timer.current)
  const hide = () => later(() => setOpen(false), hoverDelays.hideMs)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  useEffect(() => {
    if (!open) {
      return
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        window.clearTimeout(timer.current)
        setOpen(false)
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open])

  return (
    <span
      ref={anchor}
      className={`hover-card${className ? ` ${className}` : ''}`}
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocus={show}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          hide()
        }
      }}
    >
      {children(open ? id : undefined)}
      {open && createPortal(
        <Card id={id} administrationId={administrationId} contactId={contactId} name={name ?? ''} anchor={anchor} onEnter={stay} onLeave={hide} />,
        document.body,
      )}
    </span>
  )
}

/** How many documents a list holds, from its X-Total-Count, asking for a single row. */
function useCount(administrationId: string, contactId: string, list: 'sales_invoices' | 'documents') {
  return useQuery({
    queryKey: ['contact-card', administrationId, contactId, list],
    staleTime: 60_000,
    queryFn: async () => {
      const query = { filter: list === 'documents' ? `state:all,type:all,contact_id:${contactId}` : `state:all,contact_id:${contactId}`, page: '1', per_page: '1' }
      const options = { params: { path: { administration_id: administrationId }, query } }
      const { error, response } = list === 'documents'
        ? await api.GET('/api/v2/{administration_id}/documents', options)
        : await api.GET('/api/v2/{administration_id}/sales_invoices', options)
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return Number(response.headers.get('X-Total-Count') ?? 0)
    },
  })
}

type CardProps = {
  id: string
  administrationId: string
  contactId: string
  name: string
  anchor: React.RefObject<HTMLSpanElement | null>
  onEnter: () => void
  onLeave: () => void
}

/** The gap between the name and its card. */
const gap = 8

function Card({ id, administrationId, contactId, name: known, anchor, onEnter, onLeave }: CardProps) {
  const { t } = useTranslation()
  const contact = useContact(administrationId, contactId)
  const invoices = useCount(administrationId, contactId, 'sales_invoices')
  const documents = useCount(administrationId, contactId, 'documents')
  const ref = useRef<HTMLDivElement>(null)
  const [place, setPlace] = useState<CSSProperties>({ visibility: 'hidden', top: 0, left: 0 })
  const [side, setSide] = useState<'top' | 'bottom'>('top')
  const c = contact.data
  const name = (c && contactName(c)) || known

  // Above the name, as Moneybird places it, or below when there is no room above; never off the screen.
  useLayoutEffect(() => {
    const position = () => {
      const target = anchor.current?.getBoundingClientRect()
      const card = ref.current?.getBoundingClientRect()
      if (!target || !card) {
        return
      }
      const above = target.top - card.height - gap
      const top = above >= gap || target.bottom + gap + card.height > window.innerHeight ? Math.max(above, gap) : target.bottom + gap
      const left = Math.min(Math.max(target.left, gap), Math.max(window.innerWidth - card.width - gap, gap))
      setSide(top === target.bottom + gap ? 'bottom' : 'top')
      setPlace({ top, left })
    }
    position()
    window.addEventListener('scroll', position, true)
    window.addEventListener('resize', position)
    return () => {
      window.removeEventListener('scroll', position, true)
      window.removeEventListener('resize', position)
    }
  }, [anchor, c])

  const facts: [string, string][] = c
    ? ([
        [t('contacts.hoverCard.iban'), c.sepa_iban ?? ''],
        [t('contacts.hoverCard.email'), c.email || c.send_invoices_to_email || ''],
        [t('contacts.hoverCard.phone'), c.phone ?? ''],
      ] as [string, string][]).filter(([, v]) => v.trim() !== '')
    : []
  const to = `/${administrationId}/contacts/${contactId}`
  return (
    <div
      ref={ref}
      id={id}
      role="dialog"
      aria-label={name}
      className={`hover-card__content hover-card__content--${side}`}
      style={place}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      // The card is drawn apart from the page, but its clicks would still reach a row that opens a document.
      onClick={(e) => e.stopPropagation()}
    >
      <div className="hover-card__header">
        <Link className="hover-card__name" to={to}>{name}</Link>
        {c?.created_at && <span className="hover-card__meta">{t('contacts.hoverCard.created', { ago: distanceInWords(c.created_at, t) })}</span>}
      </div>
      {facts.length > 0 && (
        <dl className="hover-card__grid">
          {facts.map(([label, value]) => (
            <div key={label} className="hover-card__fact">
              <dt className="hover-card__label">{label}</dt>
              <dd className="hover-card__value">{value}</dd>
            </div>
          ))}
        </dl>
      )}
      <div className="hover-card__stats">
        <span className="hover-card__stat"><strong>{invoices.data ?? '–'}</strong> <span>{t('contacts.hoverCard.invoices')}</span></span>
        <span className="hover-card__stat"><strong>{documents.data ?? '–'}</strong> <span>{t('contacts.hoverCard.documents')}</span></span>
      </div>
    </div>
  )
}
