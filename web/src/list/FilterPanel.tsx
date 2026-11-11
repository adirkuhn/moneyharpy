import { createContext, type FormEvent, type ReactNode, useContext, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { contactName, useContact } from '../contacts/queries'
import { ContactPicker } from '../invoices/ContactPicker'
import { CaretIcon } from '../layout/icons'
import { useDisclosure } from './useDisclosure'

export type QuickFilter = { label: string; to: string; current: boolean }

const PanelContext = createContext<() => void>(() => {})

type Props = {
  /** What the list shows now, as the button's text. */
  label: string
  /** The panel's name, such as "Invoice filters". */
  title: string
  quick?: QuickFilter[]
  /** The advanced filter; made each time the panel opens, so it starts from what the list shows. */
  children?: ReactNode
}

/** The button left of a list's title that drops down its quick and advanced filters, after Moneybird. */
export function FilterPanel({ label, title, quick = [], children }: Props) {
  const { t } = useTranslation()
  const id = useId()
  const { open, toggle, close, root, button } = useDisclosure()
  return (
    <div className="filter-menu" ref={root}>
      <button ref={button} type="button" className="btn btn--secondary filter-menu__toggle" aria-expanded={open} aria-controls={id} onClick={toggle}>
        <span className="filter-menu__label">{label}</span>
        <CaretIcon />
      </button>
      {open && (
        <section id={id} className="filter-panel" role="search" aria-label={title}>
          {quick.length > 0 && (
            <div className="filter-panel__quick">
              <h2 className="filter-panel__heading">{t('list.quickFilter')}</h2>
              <ul className="filter-panel__links">
                {quick.map((q) => (
                  <li key={q.label}>
                    <Link to={q.to} aria-current={q.current ? 'true' : undefined} onClick={close}>{q.label}</Link>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <PanelContext.Provider value={close}>
            <div className="filter-panel__advanced">{children}</div>
          </PanelContext.Provider>
        </section>
      )}
    </div>
  )
}

type AdvancedProps = {
  /** What the fields hold now; the Filter button waits until it differs from what they held at first. */
  value: unknown
  onApply: () => void
  /** A control beside the Filter button, such as a link back to the list's default. */
  footer?: ReactNode
  children: ReactNode
}

/**
 * The advanced part of the panel: its fields and, under a rule, the Filter
 * button, which applies them and closes the panel. As on Moneybird the button
 * is disabled until something changed.
 */
export function AdvancedFilter({ value, onApply, footer, children }: AdvancedProps) {
  const { t } = useTranslation()
  const close = useContext(PanelContext)
  const id = useId()
  const current = JSON.stringify(value)
  const [initial] = useState(current)
  const changed = current !== initial
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!changed) {
      return
    }
    onApply()
    close()
  }
  return (
    <form className="filter-form" aria-labelledby={id} onSubmit={submit}>
      <h2 id={id} className="filter-panel__heading">{t('list.advancedFilter')}</h2>
      {children}
      <div className="filter-form__actions">
        <button type="submit" className="btn btn--primary" disabled={!changed}>{t('list.filter')}</button>
        {footer}
      </div>
    </form>
  )
}

type Option = { value: string; label: string }

/** One choice of a few, as a row of tags. */
export function RadioFilter({ legend, value, onChange, options }: { legend: string; value: string; onChange: (value: string) => void; options: Option[] }) {
  const name = useId()
  return (
    <fieldset className="filter-field">
      <legend className="filter-field__label">{legend}</legend>
      <div className="tag-options">
        {options.map((o) => (
          <label key={o.value} className={`tag-option${o.value === value ? ' is-checked' : ''}`}>
            <input type="radio" name={name} value={o.value} checked={o.value === value} onChange={() => onChange(o.value)} />
            <span>{o.label}</span>
          </label>
        ))}
      </div>
    </fieldset>
  )
}

/** One choice of many, as a compact select. */
export function SelectFilter({ label, value, onChange, options }: { label: string; value: string; onChange: (value: string) => void; options: Option[] }) {
  const id = useId()
  return (
    <div className="filter-field">
      <label className="filter-field__label" htmlFor={id}>{label}</label>
      <select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
    </div>
  )
}

/** A contact to filter on, picked by searching the contacts, or none. */
export function ContactFilter({ administrationId, contactId, onChange }: { administrationId: string; contactId: string; onChange: (id: string) => void }) {
  const { t } = useTranslation()
  const contact = useContact(administrationId, contactId || undefined)
  const name = contactId && contact.data ? contactName(contact.data) : ''
  return (
    <div className="filter-field filter-field--contact">
      <ContactPicker
        administrationId={administrationId}
        label={t('invoices.contact')}
        value={name}
        onChange={(c) => onChange(c.id ?? '')}
        action={contactId ? (
          <button type="button" className="icon-btn" aria-label={t('list.clearContact')} title={t('list.clearContact')} onClick={() => onChange('')}>×</button>
        ) : undefined}
      />
    </div>
  )
}

/** A yes or no, as a checkbox. */
export function CheckFilter({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <div className="filter-field">
      <label className="check">
        <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
        <span>{label}</span>
      </label>
    </div>
  )
}
