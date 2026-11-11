import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, unwrap } from '../api/client'
import { type ControlProps, FormRow } from '../forms/Form'
import { CaretIcon } from '../layout/icons'
import { type Contact, contactName } from '../contacts/queries'

type Props = {
  administrationId: string
  label: string
  /** The chosen contact's name, shown on the button. */
  value: string
  onChange: (contact: Contact) => void
  errors?: string[]
  /** A control beside the picker, such as a button that clears it. */
  action?: React.ReactNode
}

/** The contacts matching what is typed, asked of the server after a pause in typing. */
function useContactSearch(administrationId: string, query: string, enabled: boolean) {
  const [debounced, setDebounced] = useState(query)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 200)
    return () => clearTimeout(timer)
  }, [query])
  return useQuery({
    queryKey: ['contact-search', administrationId, debounced],
    enabled,
    placeholderData: keepPreviousData,
    queryFn: () =>
      unwrap(api.GET('/api/v2/{administration_id}/contacts', { params: { path: { administration_id: administrationId }, query: { per_page: '20', ...(debounced ? { query: debounced } : {}) } } })),
  })
}

/** Picks a contact by searching all of them, like the searchable selects elsewhere. */
export function ContactPicker({ administrationId, label, value, onChange, errors, action }: Props) {
  const { t } = useTranslation()
  return (
    <FormRow label={label} errors={errors} action={action}>
      {({ labelId, ...control }) => (
        <ContactCombobox administrationId={administrationId} labelledBy={labelId} control={control} onChange={onChange}>
          <span className="picker__value">{value || t('invoices.chooseContact')}</span>
          <CaretIcon />
        </ContactCombobox>
      )}
    </FormRow>
  )
}

type ComboboxProps = {
  administrationId: string
  /** Names the combobox by the id of its label, or by label itself when it has none on screen. */
  labelledBy?: string
  label?: string
  control: ControlProps
  onChange: (contact: Contact) => void
  className?: string
  /** What the button shows, such as the chosen contact. */
  children: ReactNode
}

/** The button that opens the contact search, and the search; its look is up to the caller. */
export function ContactCombobox({ administrationId, labelledBy, label, control, onChange, className = 'picker__button', children }: ComboboxProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const found = useContactSearch(administrationId, query, open)
  const options = found.data ?? []
  const { id } = control

  useEffect(() => {
    if (!open) {
      return
    }
    const onClick = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('click', onClick)
    return () => document.removeEventListener('click', onClick)
  }, [open])

  const close = () => {
    setOpen(false)
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
  const onKeyDown = (e: React.KeyboardEvent) => {
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
      e.preventDefault()
      close()
    }
  }
  const naming = labelledBy ? { 'aria-labelledby': labelledBy } : { 'aria-label': label }

  return (
    <div className="picker" ref={root}>
      <button
        ref={button}
        type="button"
        role="combobox"
        className={className}
        {...naming}
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-controls={`${id}-list`}
        {...control}
        onClick={() => setOpen(!open)}
      >
        {children}
      </button>
      {open && (
        <div className="picker__panel">
          <input
            type="search"
            aria-label={t('invoices.findContact')}
            placeholder={t('invoices.findContact')}
            autoFocus
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setActive(0)
            }}
            onKeyDown={onKeyDown}
          />
          <ul id={`${id}-list`} role="listbox" {...naming}>
            {options.map((c, index) => (
              <li key={c.id} role="option" aria-selected={index === active} className={index === active ? 'is-active' : ''} onClick={() => pick(c)}>
                {contactName(c)}
              </li>
            ))}
          </ul>
          {found.data && options.length === 0 && <p className="picker__empty">{t('invoices.noContacts')}</p>}
        </div>
      )}
    </div>
  )
}
