import { type KeyboardEvent, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { CompanyNameField } from '../contacts/CompanyNameField'
import { SegmentedRow } from '../contacts/ContactFormParts'
import { type Contact, type ContactChanges, contactName, useSaveContact } from '../contacts/queries'
import { type ControlProps, FormRow, formErrors, TextInput } from '../forms/Form'
import type { Confidence } from '../forms/Remark'
import { CaretIcon, ListIcon } from '../layout/icons'
import { PageIcon } from '../layout/pageIcons'
import { useOnboardingOptions } from '../onboarding/queries'
import { countryName } from '../settings/format'
import { HoverCard } from './ContactHoverCard'
import { useContactSearch } from './ContactPicker'
import { needsVatId } from './details'

type Props = {
  administrationId: string
  /** The chosen contact's name; empty shows the placeholder. */
  value: string
  errors?: string[]
  /** The country new contacts start in: the administration's. */
  homeCountry: string
  /** Where the full Add contact page comes back to, with the new contact. */
  returnTo: string
  onChange: (contact: Contact) => void
  /** The chosen contact of a saved invoice, whose card shows while it is hovered, as on Moneybird's Edit invoice. */
  hoverCard?: { id: string; name: string }
  /** Scan and Recognise chose the contact. */
  remark?: Confidence
  /**
   * Moneybird's contact on a document's page: the name as plain text, which
   * opens the search under it, with its own words for the search and the add button.
   */
  plain?: { searchPlaceholder: string; addLabel: string }
  /** While the contact picked is being saved. */
  busy?: boolean
}

/**
 * Moneybird's contact selector of the "Add invoice" popup: a search of all
 * contacts, and a short form to add a new one without leaving the invoice.
 */
export function ContactSelect({ administrationId, value, errors, homeCountry, returnTo, onChange, hoverCard, remark, plain, busy }: Props) {
  const { t } = useTranslation()
  return (
    <FormRow label={t('invoices.contact')} errors={errors} remark={remark}>
      {({ labelId, ...control }) => (
        <Selector
          administrationId={administrationId} labelId={labelId} control={control} value={value} homeCountry={homeCountry} returnTo={returnTo}
          onChange={onChange} hoverCard={hoverCard} plain={plain} busy={busy}
        />
      )}
    </FormRow>
  )
}

type SelectorProps = Omit<Props, 'errors' | 'remark'> & { labelId: string; control: ControlProps }

function Selector({ administrationId, labelId, control, value, homeCountry, returnTo, onChange, hoverCard, plain, busy }: SelectorProps) {
  const { t } = useTranslation()
  const [pane, setPane] = useState<'closed' | 'select' | 'add'>('closed')
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const found = useContactSearch(administrationId, query, pane === 'select')
  const options = found.data ?? []
  const listId = `${control.id}-list`

  useEffect(() => {
    if (pane !== 'select') {
      return
    }
    search.current?.focus()
    const onClick = (e: MouseEvent) => {
      // A click that just replaced the pane, such as Cancel of the add form, is not outside it.
      const target = e.target as Node
      if (target.isConnected && !root.current?.contains(target)) {
        setPane('closed')
      }
    }
    document.addEventListener('click', onClick)
    return () => document.removeEventListener('click', onClick)
  }, [pane])

  const close = () => {
    setPane('closed')
    setQuery('')
    setActive(0)
    button.current?.focus()
  }
  const pick = (contact: Contact | undefined) => {
    if (contact) {
      onChange(contact)
      close()
    }
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((i) => Math.min(i + 1, options.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      pick(options[active])
    } else if (e.key === 'Escape') {
      // Only the selector closes, not the popup it is in.
      e.preventDefault()
      e.stopPropagation()
      close()
    }
  }

  const trigger = (describedBy?: string) => (
    <button
      ref={button}
      type="button"
      role="combobox"
      className={plain ? 'picker__button contact-select__plain' : 'picker__button'}
      aria-labelledby={labelId}
      aria-busy={busy || undefined}
      aria-expanded={pane !== 'closed'}
      aria-haspopup="listbox"
      aria-controls={listId}
      {...control}
      aria-describedby={[control['aria-describedby'], describedBy].filter(Boolean).join(' ') || undefined}
      onClick={() => setPane(pane === 'closed' ? 'select' : 'closed')}
    >
      <span className={`picker__value${value ? '' : ' is-placeholder'}`}>{value || t('invoices.popup.selectContact')}</span>
      {!plain && <CaretIcon />}
    </button>
  )

  return (
    <div className="contact-select" ref={root}>
      {hoverCard && pane === 'closed'
        ? (
          <HoverCard className="hover-card--block" administrationId={administrationId} contact={{ id: hoverCard.id, company_name: hoverCard.name, firstname: null, lastname: null, address1: null, address2: null, zipcode: null, city: null }}>
            {trigger}
          </HoverCard>
        )
        : trigger()}
      {pane === 'select' && (
        <div className="contact-select__panel">
          <div className="contact-select__search">
            <input
              type="search"
              aria-label={t('invoices.popup.query')}
              placeholder={plain?.searchPlaceholder ?? t('invoices.popup.searchContacts')}
              ref={search}
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setActive(0)
              }}
              onKeyDown={onKeyDown}
            />
            <button type="button" className="btn btn--secondary btn--with-icon" onClick={() => setPane('add')}>
              <ListIcon kind="plus" />
              <span>{plain?.addLabel ?? t('invoices.popup.addContact')}</span>
            </button>
          </div>
          <ul id={listId} role="listbox" aria-labelledby={labelId} className="contact-select__list">
            {options.map((c, index) => (
              <li key={c.id} role="option" aria-selected={index === active} className={index === active ? 'is-active' : ''} onClick={() => pick(c)}>
                {contactName(c)}
              </li>
            ))}
          </ul>
          {found.data && options.length === 0 && <p className="picker__empty">{t('invoices.popup.noContacts')}</p>}
        </div>
      )}
      {pane === 'add' && (
        <QuickContact
          administrationId={administrationId}
          homeCountry={homeCountry}
          returnTo={returnTo}
          onCancel={() => setPane('select')}
          onSaved={pick}
        />
      )}
    </div>
  )
}

type Kind = 'company' | 'private_individual'
const fields = ['company_name', 'firstname', 'lastname', 'address1', 'zipcode', 'city', 'send_invoices_to_email', 'tax_number'] as const
type Field = (typeof fields)[number]

/** What the quick form hands to the full Add contact page, so nothing typed is lost. */
export type ContactPrefill = Partial<Record<Field | 'country', string>> & { kind?: Kind }

type QuickProps = {
  administrationId: string
  homeCountry: string
  returnTo: string
  onCancel: () => void
  onSaved: (contact: Contact) => void
}

/** The "Add contact" pane: the few fields an invoice needs, saved as a new contact. */
function QuickContact({ administrationId, homeCountry, returnTo, onCancel, onSaved }: QuickProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveContact(administrationId)
  const options = useOnboardingOptions()
  const [kind, setKind] = useState<Kind>('company')
  const [country, setCountry] = useState(homeCountry)
  const [text, setText] = useState<Record<Field, string>>(() => Object.fromEntries(fields.map((f) => [f, ''])) as Record<Field, string>)
  const countries = useMemo(
    () => [...new Set([...(options.data?.countries ?? []), country])].map((value) => ({ value, label: countryName(i18n.language, value) })).sort((a, b) => a.label.localeCompare(b.label, i18n.language)),
    [options.data, country, i18n.language],
  )
  const f = (key: string) => t(`contacts.form.${key}`)
  const labels: Record<string, string> = {
    company_name: f('companyName'), firstname: f('firstname'), lastname: f('lastname'), address1: f('address'), zipcode: f('zipcode'), city: f('city'),
    country: f('country'), send_invoices_to_email: t('invoices.popup.invoiceEmail'), tax_number: t('invoices.popup.vatId'),
  }
  const errors = formErrors(save.error, labels)
  const vatId = kind === 'company' && needsVatId(country, homeCountry)
  const change = (field: Field, value: string) => setText((current) => ({ ...current, [field]: value }))
  const input = (field: Field, extra: Partial<React.ComponentProps<typeof TextInput>> = {}) => (
    <TextInput label={labels[field] ?? field} maxLength={255} autoComplete="off" value={text[field]} errors={errors.field(field)} onChange={(e) => change(field, e.target.value)} {...extra} />
  )

  const shown: Field[] = [...(kind === 'company' ? ['company_name' as const] : ['firstname' as const, 'lastname' as const]), 'address1', 'zipcode', 'city', 'send_invoices_to_email', ...(vatId ? ['tax_number' as const] : [])]
  const submit = () => {
    const body: ContactChanges = { country }
    for (const field of shown) {
      const v = text[field].trim()
      if (v) {
        body[field] = v
      }
    }
    save.mutate(body, { onSuccess: onSaved })
  }
  const complete = () => {
    const prefill: ContactPrefill = { kind, country, ...Object.fromEntries(shown.map((field) => [field, text[field].trim()])) }
    navigate(`/${administrationId}/contacts/new?${new URLSearchParams({ return_to: returnTo })}`, { state: { prefill } })
  }

  return (
    <section
      className="contact-select__add"
      aria-label={t('invoices.contact')}
      onKeyDown={(e) => {
        // Enter saves the contact, not the invoice around it; Escape goes back to the search.
        if (e.key === 'Enter' && !e.defaultPrevented && (e.target as HTMLElement).tagName === 'INPUT') {
          e.preventDefault()
          submit()
        } else if (e.key === 'Escape') {
          e.preventDefault()
          e.stopPropagation()
          onCancel()
        }
      }}
    >
      <Problems messages={errors.general} />
      <SegmentedRow<Kind> label={f('type')} value={kind} onChange={setKind}
        options={[{ value: 'company', label: f('company') }, { value: 'private_individual', label: f('private') }]} />
      {kind === 'company' ? (
        <CompanyNameField
          label={labels.company_name ?? ''}
          value={text.company_name}
          errors={errors.field('company_name')}
          registers={options.data?.company_search_countries ?? []}
          countries={countries}
          country={country}
          onCountry={setCountry}
          onChange={(value) => change('company_name', value)}
          onChoose={(company) => setText((current) => ({
            ...current, company_name: company.name, address1: company.address1 || current.address1, zipcode: company.zipcode || current.zipcode, city: company.city || current.city,
          }))}
        />
      ) : (
        <>
          {input('firstname')}
          {input('lastname')}
        </>
      )}
      {input('address1')}
      {input('zipcode', { width: 'small' })}
      {input('city', { width: 'medium' })}
      {input('send_invoices_to_email', { type: 'email' })}
      {vatId && input('tax_number', { width: 'medium' })}
      <div className="contact-select__actions">
        <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
        <button type="button" className="btn btn--secondary btn--with-icon" onClick={complete}>
          <PageIcon kind="user" />
          <span>{t('invoices.popup.completeContact')}</span>
        </button>
        <button type="button" className="btn btn--primary" disabled={save.isPending} onClick={submit}>{t('contacts.form.save')}</button>
      </div>
    </section>
  )
}
