import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { TextInput } from '../forms/Form'
import { ContactIcon } from './ContactIcon'
import { useVatLookup, type VatMatch } from './queries'

/** The VAT number prefixes VIES answers for: the EU member states and Northern Ireland. */
const viesPrefixes = /^(AT|BE|BG|CY|CZ|DE|DK|EE|EL|ES|FI|FR|HR|HU|IE|IT|LT|LU|LV|MT|NL|PL|PT|RO|SE|SI|SK|XI)[0-9A-Z+*]{2,12}$/

/** A VAT number without the spaces, dots and dashes it is printed with, in capitals. */
export function normalizeVat(value: string) {
  return value.replace(/[\s.-]/g, '').toUpperCase()
}

type Props = {
  administrationId: string
  label: string
  value: string
  errors?: string[]
  labelExtra?: ReactNode
  /** Whether VAT numbers can be looked up in VIES; off leaves a plain field. */
  lookup: boolean
  onChange: (value: string) => void
  /** Shown under the field, such as a contact that already has the number. */
  warning?: ReactNode
  /** Called on leaving the field, with the number as typed. */
  onLeave?: (value: string) => void
  /** Fills in the contact from what VIES registered; tells whether it filled in anything. */
  onFound: (company: VatMatch) => boolean
}

/**
 * The VAT number, which an EU VAT number can fill the contact from: on
 * leaving the field, or with Look up, VIES is asked for the registered name
 * and address. Only the VAT number is sent, and only while an administrator
 * turned the lookup on.
 */
export function TaxNumberField({ administrationId, label, value, errors, labelExtra, lookup: enabled, warning, onChange, onLeave, onFound }: Props) {
  const { t } = useTranslation()
  const lookup = useVatLookup(administrationId)
  const [asked, setAsked] = useState('')
  const [filled, setFilled] = useState(false)
  const number = normalizeVat(value)
  const lookable = enabled && viesPrefixes.test(number)

  const ask = (again: boolean) => {
    if (!lookable || lookup.isPending || (!again && number === asked)) {
      return
    }
    setAsked(number)
    lookup.mutate(number, { onSuccess: (found) => setFilled((before) => (found.valid && onFound(found)) || before) })
  }
  // Problems are told quietly, under the field, and only for the number as it was asked.
  const current = enabled && number === asked && !lookup.isPending
  const problem = !current ? undefined : lookup.isError ? t(vatProblem(lookup.error)) : lookup.data && !lookup.data.valid ? t('contacts.form.vatNotFound') : undefined

  return (
    <>
      <TextInput
        label={label}
        width="medium"
        maxLength={255}
        autoComplete="off"
        value={value}
        errors={errors}
        labelExtra={labelExtra}
        help={problem && <span role="status">{problem}</span>}
        onChange={(e) => onChange(e.target.value)}
        onBlur={() => {
          ask(false)
          onLeave?.(value)
        }}
        action={lookable ? (
          <button type="button" className="contact-form__lookup" disabled={lookup.isPending} onClick={() => ask(true)}>
            {t('contacts.form.vatLookup')}
          </button>
        ) : undefined}
      />
      {warning}
      {enabled && filled && (
        <p className="contact-card__notice contact-form__notice" role="status">
          <ContactIcon kind="info" />
          <span>{t('contacts.form.viesFilled')}</span>
        </p>
      )}
    </>
  )
}

type Problem = 'contacts.form.viesUnavailable' | 'contacts.form.vatInvalid' | 'contacts.form.viesOff'

function vatProblem(error: unknown): Problem {
  if (error instanceof ApiError && error.status === 422) {
    return 'contacts.form.vatInvalid'
  }
  if (error instanceof ApiError && error.code === 'vies_disabled') {
    return 'contacts.form.viesOff'
  }
  return 'contacts.form.viesUnavailable'
}
