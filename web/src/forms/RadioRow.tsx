import { useId } from 'react'

type Option<T extends string> = { value: T; label: string }

type Props<T extends string> = {
  label: string
  value: T
  options: Option<T>[]
  errors?: string[]
  onChange: (value: T) => void
}

/** A labelled row of radio buttons, for a short choice such as a language. */
export function RadioRow<T extends string>({ label, value, options, errors, onChange }: Props<T>) {
  const id = useId()
  const invalid = errors !== undefined && errors.length > 0
  return (
    <div className={`form-input${invalid ? ' form-input--invalid' : ''}`}>
      <span className="form-input__label" id={`${id}-label`}>{label}</span>
      <div className="form-input__body">
        <div className="radio-row" role="radiogroup" aria-labelledby={`${id}-label`} {...(invalid ? { 'aria-describedby': `${id}-message` } : {})}>
          {options.map((o) => (
            <label key={o.value}>
              <input type="radio" name={id} value={o.value} checked={value === o.value} onChange={() => onChange(o.value)} />
              <span>{o.label}</span>
            </label>
          ))}
        </div>
        {invalid && <p id={`${id}-message`} className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
      </div>
    </div>
  )
}
