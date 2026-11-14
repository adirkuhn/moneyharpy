import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { formatAmount } from '../banking/queries'
import { Loading } from '../layout/Loading'
import { ContactIcon } from './ContactIcon'
import { ContactName } from './ContactHoverCard'
import { amountSign, type ContactDocuments, documentPath, type FeedEntry, newestFirst } from './documents'
import { type Contact, contactName } from './queries'

type Props = { administrationId: string; contact: Contact; documents: ContactDocuments[]; pending: boolean }

/**
 * The contact's feed, after Moneybird's: what still needs doing, or that
 * everything has been processed, and then the documents processed lately.
 */
export function ContactFeed({ administrationId, contact, documents, pending }: Props) {
  const { t } = useTranslation()
  const recentId = useId()
  if (pending) {
    return <Loading />
  }
  const entries = newestFirst(documents.flatMap((d) => d.entries))
  const open = entries.filter((e) => e.open)
  const processed = entries.filter((e) => !e.open)
  const row = (e: FeedEntry) => <li key={`${e.kind}-${e.id}`}><FeedRow administrationId={administrationId} contact={contact} entry={e} /></li>
  return (
    <div className="contact-feed">
      <div className="contact-feed__top">
        {open.length > 0 ? (
          <ul className="contact-feed__list" aria-label={t('contacts.view.toProcess')}>{open.map(row)}</ul>
        ) : (
          <div className="contact-feed__done">
            <DoneIllustration />
            <h2 className="contact-feed__done-title">{t('contacts.view.allProcessed')}</h2>
          </div>
        )}
      </div>
      <div className="contact-feed__recent">
        <h2 id={recentId} className="contact-feed__subtitle">{t('contacts.view.recent')}</h2>
        {processed.length > 0
          ? <ul className="contact-feed__list" aria-labelledby={recentId}>{processed.map(row)}</ul>
          : <p className="contact-feed__empty">{t('contacts.view.noRecent')}</p>}
      </div>
    </div>
  )
}

const icons = { invoice: 'invoice', external: 'external', quote: 'quote', purchase: 'purchase', receipt: 'receipt' } as const

function FeedRow({ administrationId, contact, entry: e }: { administrationId: string; contact: Contact; entry: FeedEntry }) {
  const { t, i18n } = useTranslation()
  const kind = t(`contacts.view.kinds.${e.kind}`)
  const names = e.names.length > 0 ? e.names : [kind]
  const title = [contactName(contact), ...names].join(' · ')
  const sign = amountSign(e.kind)
  const value = Number(e.amount) * (sign === '-' ? -1 : 1)
  const amount = signedAmount(value, sign !== '', e.currency || 'EUR', i18n.language)
  const state = e.kind === 'quote' ? t(`estimates.states.${e.state}`, { defaultValue: e.state }) : t(`invoices.states.${e.state}`, { defaultValue: e.state })
  return (
    <Link className="feed-row" to={documentPath(administrationId, e)} aria-label={`${kind}: ${title}`}>
      <span className="feed-row__icon" title={kind}><ContactIcon kind={icons[e.kind]} /></span>
      <span className="feed-row__title">
        <ContactName administrationId={administrationId} contactId={contact.id} name={contactName(contact)} />
        {names.map((n) => ` · ${n}`).join('')}
      </span>
      <span className="feed-row__amount">{amount}</span>
      <span className="feed-row__subtitle">
        {e.state && <span className={`badge badge--${e.state}`}>{state}</span>}
        {e.lines.length > 0 && <span className="feed-row__lines">{e.lines.join(' · ')}</span>}
      </span>
      <span className="feed-row__date">{shortDate(e.date, i18n.language)}</span>
    </Link>
  )
}

/** "Sep 16" for this year, "Sep 11, 2025" for another. */
function shortDate(date: string, language: string) {
  if (!date) {
    return ''
  }
  const d = new Date(`${date.slice(0, 10)}T00:00:00`)
  const thisYear = d.getFullYear() === new Date().getFullYear()
  return d.toLocaleDateString(language, { month: 'short', day: 'numeric', ...(thisYear ? {} : { year: 'numeric' }) })
}

/** A finished stack of papers with a tick and some confetti, drawn for MoneyHarpy. */
function DoneIllustration() {
  return (
    <svg className="contact-feed__illustration" viewBox="0 0 200 150" width="200" height="150" aria-hidden="true">
      <rect x="62" y="34" width="76" height="96" rx="8" transform="rotate(-8 100 82)" fill="var(--blue)" opacity="0.18" />
      <rect x="62" y="30" width="76" height="96" rx="8" fill="var(--surface)" stroke="var(--blue)" strokeWidth="3" />
      <path d="M76 52h48M76 64h48M76 76h30" stroke="var(--line)" strokeWidth="5" strokeLinecap="round" />
      <circle cx="128" cy="104" r="24" fill="var(--teal)" />
      <path d="m117 104 8 8 15-16" fill="none" stroke="var(--white)" strokeWidth="5" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="44" cy="40" r="5" fill="var(--gold)" />
      <path d="M156 30l10-4M150 18l3-10" stroke="var(--teal)" strokeWidth="4" strokeLinecap="round" />
      <path d="M36 92l-8 6 10 2Z" fill="var(--blue)" />
      <path d="M40 64c4-6 10 0 14-6" fill="none" stroke="var(--navy)" strokeWidth="2" strokeLinecap="round" opacity="0.6" />
      <rect x="164" y="64" width="12" height="5" rx="2" transform="rotate(-20 170 66)" fill="var(--gold)" />
    </svg>
  )
}

/**
 * An amount with its sign after the currency symbol, as Moneybird writes
 * them in a feed ("€+99.21", "€-25.10"); money in carries a plus.
 */
export function signedAmount(value: number, signed: boolean, currency: string, language: string) {
  const plain = formatAmount(Math.abs(value), currency, language)
  const mark = value < 0 ? '-' : signed && value > 0 ? '+' : ''
  if (!mark) {
    return plain
  }
  const digit = plain.search(/\d/)
  return digit < 0 ? mark + plain : plain.slice(0, digit) + mark + plain.slice(digit)
}
