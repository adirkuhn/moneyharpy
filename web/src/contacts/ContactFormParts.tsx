import { type ReactNode, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { ContactIcon, type ContactIconKind } from './ContactIcon'

/** A part of Moneybird's contact form: an icon and a heading over a rule, then its rows. */
export function Section({ icon, title, action, children }: { icon: ContactIconKind; title: string; action?: ReactNode; children: ReactNode }) {
  const id = useId()
  return (
    <section className="contact-form__section" aria-labelledby={id}>
      <div className="contact-form__section-header">
        <span className="contact-form__section-icon"><ContactIcon kind={icon} /></span>
        <h2 id={id} className="contact-form__section-title">{title}</h2>
        {action && <div className="contact-form__section-action">{action}</div>}
      </div>
      <div className="contact-form__rows">{children}</div>
    </section>
  )
}

type Option<T extends string> = { value: T; label: string }

/** A short choice as joined segments, the chosen one raised, as Moneybird's Type and Delivery method. */
export function SegmentedRow<T extends string>({ label, value, options, errors, onChange }: { label: string; value: T; options: Option<T>[]; errors?: string[]; onChange: (value: T) => void }) {
  const id = useId()
  const invalid = errors !== undefined && errors.length > 0
  return (
    <div className={`form-input${invalid ? ' form-input--invalid' : ''}`}>
      <span className="form-input__label" id={`${id}-label`}>{label}</span>
      <div className="form-input__body">
        <div className="segment-control" role="radiogroup" aria-labelledby={`${id}-label`} {...(invalid ? { 'aria-describedby': `${id}-message` } : {})}>
          {options.map((o) => (
            <label key={o.value} className={`segment-control__item${o.value === value ? ' is-checked' : ''}`}>
              <input type="radio" name={id} value={o.value} checked={o.value === value} onChange={() => onChange(o.value)} />
              <span>{o.label}</span>
            </label>
          ))}
        </div>
        {invalid && <p id={`${id}-message`} className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
      </div>
    </div>
  )
}

type CheckRowProps = {
  /** The row's label on the left, such as "UBL invoice". */
  label: string
  /** The checkbox's own text, which names it. */
  text: string
  tip?: string
  muted?: boolean
  errors?: string[]
} & Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'>

/** A checkbox with a label on the left and its own text on the right, as Moneybird's contact form has them. */
export function CheckRow({ label, text, tip, muted = false, errors, ...input }: CheckRowProps) {
  const id = useId()
  const invalid = errors !== undefined && errors.length > 0
  return (
    <div className={`form-input form-input--check-row${invalid ? ' form-input--invalid' : ''}`}>
      <div className="form-input__label-row">
        <span className={`form-input__label${muted ? ' is-muted' : ''}`} title={label}>{label}</span>
        {tip && <Tip text={tip} />}
      </div>
      <div className="form-input__body">
        <label className="check">
          <input type="checkbox" {...input} {...(invalid ? { 'aria-invalid': true, 'aria-describedby': `${id}-message` } : {})} />
          <span>{text}</span>
        </label>
        {invalid && <p id={`${id}-message`} className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
      </div>
    </div>
  )
}

/** An information button beside a label; its text shows on hover and focus. */
export function Tip({ text }: { text: string }) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <span className="tip">
      <button type="button" className="tip__button" aria-label={t('list.moreInfo')} aria-describedby={id}>
        <ContactIcon kind="info" />
      </button>
      <span id={id} role="tooltip" className="tip__text">{text}</span>
    </span>
  )
}

/** How many characters of the most a field holds, inside its right edge. */
export function Counter({ length, max }: { length: number; max: number }) {
  return <span className="char-counter" aria-hidden="true">{length}/{max}</span>
}
