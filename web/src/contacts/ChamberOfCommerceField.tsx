import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { TextInput } from '../forms/Form'
import { ContactIcon } from './ContactIcon'
import { type CompanyMatch, useKvkLookup } from './queries'

/** The countries whose company registration number has a name of its own; the others say "Company registration number". */
const namedRegistries = ['NL', 'BE', 'DE', 'PT', 'FR', 'ES'] as const
type Registry = (typeof namedRegistries)[number] | 'other'

/** Which name the chamber of commerce number has in a country. */
export function registryOf(country: string): Registry {
  return (namedRegistries as readonly string[]).includes(country) ? (country as Registry) : 'other'
}

/** A KVK number as it is compared: its digits, without spaces or dots. */
export function kvkDigits(value: string) {
  return value.replace(/[\s.]/g, '')
}

/** Whether a Dutch chamber of commerce number has the eight digits of a KVK number; an empty one is fine. */
export function validKvk(value: string) {
  const digits = kvkDigits(value.trim())
  return digits === '' || /^\d{8}$/.test(digits)
}

type Props = {
  administrationId: string
  /** The contact's country, which names the number and decides whether it is a KVK number. */
  country: string
  value: string
  errors?: string[]
  /** Whether the KVK register can be asked: on for a Dutch contact while an administrator enabled KVK. */
  lookup: boolean
  /** Tells a Dutch number without eight digits before the field was left, such as when saving. */
  checkFormat?: boolean
  /** Shown under the field, such as a contact that already has the number. */
  warning?: ReactNode
  onChange: (value: string) => void
  /** Called on leaving the field, with the number as typed. */
  onLeave?: (value: string) => void
  /** Fills in the contact from the register; tells whether it filled in anything. */
  onFound: (company: CompanyMatch) => boolean
}

/**
 * The chamber of commerce number, named after the contact country's register.
 * A Dutch one must have eight digits, and when KVK is enabled it fills the
 * contact from the register on leaving the field, or with Look up. Moneybird
 * keeps every country's number in this one field.
 */
export function ChamberOfCommerceField({ administrationId, country, value, errors, lookup: enabled, checkFormat = false, warning, onChange, onLeave, onFound }: Props) {
  const { t } = useTranslation()
  const lookup = useKvkLookup(administrationId)
  const [asked, setAsked] = useState('')
  const [filled, setFilled] = useState(false)
  const [left, setLeft] = useState(false)
  const dutch = country === 'NL'
  const number = kvkDigits(value.trim())
  const lookable = dutch && enabled && /^\d{8}$/.test(number)
  const registry = registryOf(country)

  const ask = (again: boolean) => {
    if (!lookable || lookup.isPending || (!again && number === asked)) {
      return
    }
    setAsked(number)
    lookup.mutate(number, { onSuccess: (found) => setFilled((before) => onFound(found) || before) })
  }
  const formatError = dutch && (left || checkFormat) && !validKvk(value) ? [t('contacts.form.kvkFormat')] : []
  // Problems are told quietly, under the field, and only for the number as it was asked.
  const current = lookable && number === asked && !lookup.isPending && lookup.isError
  const problem = current ? t(lookup.error instanceof ApiError && lookup.error.status === 404 ? 'contacts.form.kvkNotFound' : 'contacts.form.kvkUnavailable') : undefined

  return (
    <>
      <TextInput
        label={t(`contacts.form.registry.${registry}`)}
        width="medium"
        maxLength={255}
        autoComplete="off"
        value={value}
        errors={[...(errors ?? []), ...formatError]}
        help={problem && <span role="status">{problem}</span>}
        onChange={(e) => onChange(e.target.value)}
        onBlur={() => {
          setLeft(true)
          ask(false)
          onLeave?.(value)
        }}
        action={lookable ? (
          <button type="button" className="contact-form__lookup" disabled={lookup.isPending} onClick={() => ask(true)}>
            {t('contacts.form.kvkLookup')}
          </button>
        ) : undefined}
      />
      {warning}
      {lookable && filled && (
        <p className="contact-card__notice contact-form__notice" role="status">
          <ContactIcon kind="info" />
          <span>{t('contacts.form.kvkFilled')}</span>
        </p>
      )}
    </>
  )
}
