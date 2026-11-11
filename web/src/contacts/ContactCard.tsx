import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors, TextInput } from '../forms/Form'
import { useOnboardingOptions } from '../onboarding/queries'
import { countryName } from '../settings/format'
import { ContactIcon, type ContactIconKind } from './ContactIcon'
import { type ContactDocuments, listPath } from './documents'
import { type Contact, type ContactPerson, contactName, type PersonChanges, useDeletePerson, useSavePerson } from './queries'

type Props = { administrationId: string; contact: Contact; documents: ContactDocuments[] }

/** The address of a contact as its lines: street, extra line, zipcode and city, country. */
export function addressLines(c: Contact, language: string) {
  return [c.address1, c.address2, [c.zipcode, c.city].filter(Boolean).join(' '), countryName(language, c.country ?? '')]
    .filter((line): line is string => !!line)
}

/**
 * The card on the left of a contact, after Moneybird's: its name and client
 * number, buttons to reach it, how to reach it, its documents and its people.
 */
export function ContactCard({ administrationId, contact: c, documents }: Props) {
  const { t, i18n } = useTranslation()
  const heading = useId()
  const name = contactName(c)
  const company = !!c.company_name
  const email = c.email || c.send_invoices_to_email
  const address = addressLines(c, i18n.language)
  const street = [c.address1, c.address2, c.zipcode, c.city].some(Boolean)
  const dutch = c.country === 'NL'
  const options = useOnboardingOptions()
  // Moneybird offers the register's details for a Dutch company it cannot place yet.
  const suggestRegister = company && dutch && !c.chamber_of_commerce && (options.data?.company_search_countries ?? []).includes('NL')
  const counted = documents.filter((d) => d.total > 0)

  const quick: { kind: ContactIconKind; label: string; href: string; external?: boolean }[] = [
    ...(c.phone ? [{ kind: 'phone' as const, label: t('contacts.view.call'), href: `tel:${c.phone.replace(/\s+/g, '')}` }] : []),
    ...(email ? [{ kind: 'mail' as const, label: t('contacts.view.email'), href: `mailto:${email.split(',')[0]?.trim() ?? ''}` }] : []),
    ...(street ? [{ kind: 'map' as const, label: t('contacts.view.map'), href: `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(address.join(', '))}`, external: true }] : []),
    ...(company && dutch && c.chamber_of_commerce
      ? [{ kind: 'landmark' as const, label: t('contacts.view.kvk'), href: `https://www.kvk.nl/zoeken/?source=all&q=${encodeURIComponent(c.chamber_of_commerce)}`, external: true }]
      : []),
  ]

  return (
    <section className="contact-card" aria-labelledby={heading}>
      <div className="contact-card__info">
        <h2 id={heading} className="contact-card__name">{name}</h2>
        <p className="contact-card__subtitle">{t('contacts.view.clientNumber', { id: c.customer_id })}</p>
      </div>
      {quick.length > 0 && (
        <div className="contact-card__quick">
          {quick.map((q) => (
            <a
              key={q.kind}
              className="btn btn--secondary btn--icon"
              href={q.href}
              aria-label={q.label}
              title={q.label}
              {...(q.external ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
            >
              <ContactIcon kind={q.kind} />
            </a>
          ))}
        </div>
      )}
      {suggestRegister && (
        <p className="contact-card__notice">
          <ContactIcon kind="info" />
          <span>
            {t('contacts.view.registerNotice')}{' '}
            <Link to={`/${administrationId}/contacts/${c.id}/edit`}>{t('contacts.view.registerLink')}</Link>
          </span>
        </p>
      )}
      {email && <Fact label={t('contacts.view.emailAddress')}>{email}</Fact>}
      {c.phone && <Fact label={t('contacts.view.phone')}>{c.phone}</Fact>}
      {street && <Fact label={t('contacts.view.address')}>{address.map((line) => <span key={line} className="contact-card__line">{line}</span>)}</Fact>}
      {c.chamber_of_commerce && <Fact label={dutch ? t('contacts.view.kvkNumber') : t('contacts.view.cocNumber')}>{c.chamber_of_commerce}</Fact>}
      {c.tax_number && (
        <Fact label={t('contacts.view.taxNumber')}>
          {c.tax_number} <CopyButton value={c.tax_number} label={t('contacts.view.copyTaxNumber')} />
        </Fact>
      )}
      {counted.length > 0 && (
        <Fact label={t('contacts.view.documents')}>
          {counted.map((d) => (
            <Link key={d.kind} className="contact-card__line" to={listPath(administrationId, d.kind, c.id ?? '')}>
              {t(`contacts.view.counts.${d.kind}`, { count: d.total })}
            </Link>
          ))}
        </Fact>
      )}
      <People administrationId={administrationId} contact={c} company={company} />
    </section>
  )
}

/** A small button that puts a value on the clipboard, as Moneybird offers for the VAT number. */
function CopyButton({ value, label }: { value: string; label: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const copy = () => {
    void navigator.clipboard?.writeText(value).then(() => setCopied(true), () => setCopied(false))
  }
  return (
    <button type="button" className="contact-card__copy" aria-label={label} title={copied ? t('contacts.view.copied') : label} onClick={copy}>
      <ContactIcon kind="copy" />
    </button>
  )
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <dl className="contact-card__fact">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </dl>
  )
}

/** The people of a company, under its card; Moneybird offers to add them to companies only. */
function People({ administrationId, contact, company }: { administrationId: string; contact: Contact; company: boolean }) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState<ContactPerson | 'new' | null>(null)
  const remove = useDeletePerson(administrationId, contact.id ?? '')
  const people = contact.contact_people ?? []
  const problems = formErrors(remove.error, {}).general
  return (
    <section className="contact-card__people" aria-label={t('contacts.people')}>
      <Problems messages={problems} />
      {people.length > 0 && (
        <dl className="contact-card__fact">
          <dt>{t('contacts.people')}</dt>
          <dd>
            <ul className="people">
              {people.map((p) => {
                const name = [p.firstname, p.lastname].filter(Boolean).join(' ')
                return editing !== 'new' && editing?.id === p.id ? (
                  <li key={p.id}><PersonForm administrationId={administrationId} contactId={contact.id ?? ''} person={p} onDone={() => setEditing(null)} /></li>
                ) : (
                  <li key={p.id} className="people__row">
                    <div>
                      <p className="people__name">{name}</p>
                      {[p.department, p.email, p.phone].filter(Boolean).map((detail) => <p key={detail} className="hint">{detail}</p>)}
                    </div>
                    <div className="people__actions">
                      <button type="button" className="btn btn--secondary btn--small" aria-label={t('contacts.view.editPerson', { name })} onClick={() => setEditing(p)}>
                        {t('contacts.edit')}
                      </button>
                      <ConfirmDelete label={t('contacts.deletePerson', { name })} confirmLabel={t('contacts.confirmDeletePerson', { name })} text={t('contacts.remove')} confirmText={t('contacts.confirmRemove')} disabled={remove.isPending} onConfirm={() => remove.mutate(String(p.id ?? ''))} />
                    </div>
                  </li>
                )
              })}
            </ul>
          </dd>
        </dl>
      )}
      {editing === 'new' && <PersonForm administrationId={administrationId} contactId={contact.id ?? ''} onDone={() => setEditing(null)} />}
      {company && editing === null && (
        <button type="button" className="btn btn--secondary btn--with-icon contact-card__add" onClick={() => setEditing('new')}>
          <ContactIcon kind="plus" />
          <span>{t('contacts.addPerson')}</span>
        </button>
      )}
    </section>
  )
}

const fields = ['firstname', 'lastname', 'email', 'phone', 'department'] as const

type PersonFormProps = { administrationId: string; contactId: string; person?: ContactPerson; onDone: () => void }

/** Adds a contact person, or changes one. */
function PersonForm({ administrationId, contactId, person, onDone }: PersonFormProps) {
  const { t } = useTranslation()
  const save = useSavePerson(administrationId, contactId, person?.id ? String(person.id) : undefined)
  const [form, setForm] = useState<Required<PersonChanges>>(() => ({
    firstname: person?.firstname ?? '', lastname: person?.lastname ?? '', email: person?.email ?? '', phone: person?.phone ?? '', department: person?.department ?? '',
  }))
  const labels = { firstname: t('contacts.firstname'), lastname: t('contacts.lastname'), email: t('contacts.email'), phone: t('contacts.phone'), department: t('contacts.department') }
  const errors = formErrors(save.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(form, { onSuccess: onDone })
  }
  return (
    <form className="form-sheet contact-card__form" onSubmit={submit} noValidate>
      <Problems messages={errors.general} />
      {fields.map((field) => (
        <TextInput key={field} label={labels[field]} maxLength={255} type={field === 'email' ? 'email' : 'text'} value={form[field]} errors={errors.field(field)} onChange={(e) => setForm({ ...form, [field]: e.target.value })} />
      ))}
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={save.isPending}>{t('contacts.savePerson')}</button>
        <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
      </div>
    </form>
  )
}
