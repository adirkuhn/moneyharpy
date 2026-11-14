import { type KeyboardEvent, useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FormRow } from '../forms/Form'
import { registerProblem } from '../onboarding/registerProblem'
import { countryName } from '../settings/format'
import { type CompanyMatch, useCompanyRegister } from './queries'

type Props = {
  label: string
  value: string
  errors?: string[]
  /** The countries whose company register can be searched; none leaves a plain field. */
  registers: string[]
  onChange: (value: string) => void
  onChoose: (company: CompanyMatch, country: string) => void
  /**
   * The contact's own country, chosen in front of the name from all of these,
   * as Moneybird's quick contact form does; the register is searched only in
   * the countries that have one. Without it the select lists the registers.
   */
  countries?: { value: string; label: string }[]
  country?: string
  /** The name of that country select; "Country" when none is given. */
  countryLabel?: string
  onCountry?: (country: string) => void
  /** Show country codes instead of names, for a box too narrow for names. */
  countryCodes?: boolean
}

const searchDelayMs = 300

/**
 * The company name, after Moneybird: a country for the company register in
 * front, and while typing the register's matches to fill the contact from.
 */
export function CompanyNameField({ label, value, errors, registers, onChange, onChoose, countries, country: own = '', countryLabel, onCountry, countryCodes = false }: Props) {
  const { t, i18n } = useTranslation()
  const listId = useId()
  const [chosen, setCountry] = useState('')
  // The registers arrive after the form; until one is chosen the Dutch one goes first, as on Moneybird.
  const fallback = registers.includes(chosen) ? chosen : registers.includes('NL') ? 'NL' : (registers[0] ?? '')
  const country = countries ? (registers.includes(own) ? own : '') : fallback
  const [term, setTerm] = useState('')
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const matches = useCompanyRegister(country, term, open && country !== '')
  const shown = open ? (matches.data ?? []) : []

  useEffect(() => {
    const timer = setTimeout(() => setTerm(value.trim()), searchDelayMs)
    return () => clearTimeout(timer)
  }, [value])
  useEffect(() => setActive(0), [term])

  const choose = (company: CompanyMatch) => {
    onChoose(company, country)
    setOpen(false)
  }
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (shown.length === 0) {
      return
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((active + (e.key === 'ArrowDown' ? 1 : shown.length - 1)) % shown.length)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const company = shown[active]
      if (company) {
        choose(company)
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setOpen(false)
    }
  }

  return (
    <FormRow
      label={label}
      errors={errors}
      // When the register cannot be searched the name stays as typed, so the rest is filled in by hand.
      help={matches.isError ? <span role="status">{t(registerProblem(matches.error))}</span> : undefined}
    >
      {({ labelId: _, ...control }) => (
        <div className="company-name">
          {countries && (
            <select className="company-name__country" aria-label={countryLabel ?? t('contacts.form.country')} value={own} onChange={(e) => onCountry?.(e.target.value)}>
              {/* A narrow box shows the code, as beside the register search; the name is its tooltip. */}
              {countries.map((c) => <option key={c.value} value={c.value} title={c.label}>{countryCodes ? c.value : c.label}</option>)}
            </select>
          )}
          {!countries && registers.length > 0 && (
            <select className="company-name__country" aria-label={t('contacts.form.registerCountry')} value={country} onChange={(e) => setCountry(e.target.value)}>
              {registers.map((c) => <option key={c} value={c} title={countryName(i18n.language, c)}>{c}</option>)}
            </select>
          )}
          <input
            type="text"
            maxLength={255}
            autoComplete="off"
            {...control}
            {...(country !== '' ? { role: 'combobox', 'aria-expanded': shown.length > 0, 'aria-controls': listId, 'aria-autocomplete': 'list' as const } : {})}
            {...(shown.length > 0 ? { 'aria-activedescendant': `${listId}-${active}` } : {})}
            value={value}
            onChange={(e) => {
              onChange(e.target.value)
              setOpen(true)
            }}
            onKeyDown={onKey}
            onBlur={() => setOpen(false)}
          />
          {shown.length > 0 && (
            <ul id={listId} className="company-name__matches" role="listbox" aria-label={t('contacts.form.registerMatches')}>
              {shown.map((c, i) => (
                <li
                  key={`${c.chamber_of_commerce}-${i}`}
                  id={`${listId}-${i}`}
                  role="option"
                  aria-selected={i === active}
                  className={i === active ? 'is-active' : undefined}
                  // Choosing on mouse down keeps the field from losing focus first, which would close the list.
                  onMouseDown={(e) => {
                    e.preventDefault()
                    choose(c)
                  }}
                >
                  <strong>{c.name}</strong>
                  <span className="hint">{[c.chamber_of_commerce && t('onboarding.kvkNumber', { number: c.chamber_of_commerce }), [c.address1, c.city].filter(Boolean).join(', ')].filter(Boolean).join(' · ')}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </FormRow>
  )
}
