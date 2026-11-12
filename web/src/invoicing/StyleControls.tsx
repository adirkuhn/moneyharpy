import { type ReactNode, useId } from 'react'
import { Tip } from '../contacts/ContactFormParts'
import { FormRow } from '../forms/Form'

/** The controls of the document style editor's parts, after Moneybird's. */

export function FieldErrors({ messages }: { messages: string[] }) {
  return messages.length ? <p className="form-input__message form-input__message--error">{messages.join(' ')}</p> : null
}

type SegmentedProps<T extends string> = { label: string; value: T; options: Array<{ value: T; label: string; icon?: ReactNode }>; errors?: string[]; onChange: (value: T) => void }

/** A labelled row of joined buttons for a short choice, such as left, centered or right. */
export function Segmented<T extends string>({ label, value, options, errors, onChange }: SegmentedProps<T>) {
  const name = useId()
  return (
    <fieldset className="segmented style-segmented">
      <legend className="segmented__legend">{label}</legend>
      <div className="segmented__options">
        {options.map((o) => (
          <label key={o.value} className={`segmented__option${value === o.value ? ' is-checked' : ''}`} title={o.label}>
            <input type="radio" name={name} value={o.value} checked={value === o.value} aria-label={o.label} onChange={() => onChange(o.value)} />
            {o.icon ?? o.label}
          </label>
        ))}
      </div>
      {errors && errors.length > 0 && <p className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
    </fieldset>
  )
}

/** An on-off switch with its label on the left, as in Moneybird's style editor. */
export function Toggle({ label, tip, checked, disabled, onChange }: { label: string; tip?: string; checked: boolean; disabled?: boolean; onChange: (on: boolean) => void }) {
  return (
    <div className="style-toggle">
      <span className="style-toggle__label">
        <label>
          <span>{label}</span>
          <input type="checkbox" role="switch" className="switch" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
        </label>
        {tip && <Tip text={tip} />}
      </span>
    </div>
  )
}

/** A slider with its value beside it, such as a width in percent. */
export function Range({ label, value, min, max, unit, errors, help, onChange }: { label: string; value: number; min: number; max: number; unit: string; errors?: string[]; help?: string; onChange: (value: number) => void }) {
  return (
    <FormRow label={label} errors={errors} help={help}>
      {({ labelId: _, ...control }) => (
        <span className="style-range">
          <input type="range" min={min} max={max} step={1} value={value} onChange={(e) => onChange(Number(e.target.value))} {...control} />
          <output aria-hidden="true">{value}{unit}</output>
        </span>
      )}
    </FormRow>
  )
}

/** A colour as Moneybird's editor asks for it: a swatch to pick with and the hex code to type. */
export function ColorField({ label, value, errors, onChange }: { label: string; value: string; errors?: string[]; onChange: (value: string) => void }) {
  const valid = /^#[0-9A-Fa-f]{6}$/.test(value)
  return (
    <FormRow label={label} errors={errors}>
      {({ labelId: _, ...control }) => (
        <span className="style-color">
          <input type="color" value={valid ? value.toLowerCase() : '#000000'} aria-hidden="true" tabIndex={-1} onChange={(e) => onChange(e.target.value.toUpperCase())} />
          <input type="text" value={value} maxLength={7} spellCheck={false} {...control} onChange={(e) => onChange(e.target.value)} />
        </span>
      )}
    </FormRow>
  )
}

export function AlignIcon({ align }: { align: 'left' | 'center' | 'right' }) {
  const lines = { left: 'M4 6h16M4 10h10M4 14h16M4 18h10', center: 'M4 6h16M7 10h10M4 14h16M7 18h10', right: 'M4 6h16M10 10h10M4 14h16M10 18h10' }
  return (
    <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" aria-hidden="true">
      <path d={lines[align]} />
    </svg>
  )
}
