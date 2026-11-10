import { type ReactNode, useId } from 'react'
import { ApiError, errorMessages } from '../api/client'
import { serverMessage } from '../i18n'
import { InfoIcon } from '../layout/icons'

/** How wide a control is, after Moneybird: short codes are small, text is long. */
export type Width = 'small' | 'medium' | 'long'

/** Props a row hands to its control so the label and messages reach it. */
export type ControlProps = {
  id: string
  'aria-invalid'?: true
  'aria-describedby'?: string
}

type RowProps = {
  label: string
  errors?: string[]
  help?: ReactNode
  width?: Width
  /** A small button placed right after the control, such as "add a line". */
  action?: ReactNode
}

type FormRowProps = RowProps & {
  /** Renders the control, given the props that tie it to the label and messages. */
  children: (control: ControlProps & { labelId: string }) => ReactNode
}

/**
 * One labelled control: the label beside it on wide screens and above it on
 * narrow ones, with its error or help text underneath.
 */
export function FormRow({ label, errors, help, width = 'long', action, children }: FormRowProps) {
  const id = useId()
  const messageId = `${id}-message`
  const invalid = errors !== undefined && errors.length > 0
  const message = invalid ? errors.join(' ') : help
  return (
    <div className={`form-input form-input--${width}${invalid ? ' form-input--invalid' : ''}`}>
      <label className="form-input__label" id={`${id}-label`} htmlFor={id}>{label}</label>
      <div className="form-input__body">
        <div className="form-input__control">
          {children({
            id,
            labelId: `${id}-label`,
            ...(invalid ? { 'aria-invalid': true as const } : {}),
            ...(message ? { 'aria-describedby': messageId } : {}),
          })}
          {action}
        </div>
        {message && (
          <p id={messageId} className={`form-input__message form-input__message--${invalid ? 'error' : 'help'}`}>{message}</p>
        )}
      </div>
    </div>
  )
}

export function TextInput({ label, errors, help, width, action, ...input }: RowProps & React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <FormRow label={label} errors={errors} help={help} width={width} action={action}>
      {({ labelId: _, ...control }) => <input type="text" {...input} {...control} />}
    </FormRow>
  )
}

export function TextArea({ label, errors, help, width, ...textarea }: RowProps & React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <FormRow label={label} errors={errors} help={help} width={width}>
      {({ labelId: _, ...control }) => <textarea rows={3} {...textarea} {...control} />}
    </FormRow>
  )
}

export function SelectInput({ label, errors, help, width, children, ...select }: RowProps & React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <FormRow label={label} errors={errors} help={help} width={width}>
      {({ labelId: _, ...control }) => (
        <select {...select} {...control}>
          {children}
        </select>
      )}
    </FormRow>
  )
}

/** A checkbox in the control column, named by its own text. */
export function CheckboxInput({ label, errors, help, ...input }: Omit<RowProps, 'width' | 'action'> & React.InputHTMLAttributes<HTMLInputElement>) {
  const id = useId()
  const invalid = errors !== undefined && errors.length > 0
  const message = invalid ? errors.join(' ') : help
  return (
    <div className="form-input form-input--check">
      <span className="form-input__label" aria-hidden="true" />
      <div className="form-input__body">
        <label className="check">
          <input
            type="checkbox"
            {...input}
            {...(invalid ? { 'aria-invalid': true } : {})}
            {...(message ? { 'aria-describedby': `${id}-message` } : {})}
          />
          <span>{label}</span>
        </label>
        {message && (
          <p id={`${id}-message`} className={`form-input__message form-input__message--${invalid ? 'error' : 'help'}`}>{message}</p>
        )}
      </div>
    </div>
  )
}

/** A group of related rows, set apart from the next group by a rule. */
export function FormSection({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <fieldset className="form-section">
      {title && <legend className="form-section__title">{title}</legend>}
      {children}
    </fieldset>
  )
}

/** A notice at the top of a form. */
export function FormNotice({ children }: { children: ReactNode }) {
  return (
    <div className="alert alert--info" role="note">
      <InfoIcon />
      <div>{children}</div>
    </div>
  )
}

export type FormErrors = {
  /** The problems to show beside a field, or undefined when it has none. */
  field: (name: string) => string[] | undefined
  /** The problems that belong to no shown field. */
  general: string[]
}

/**
 * Splits an API error over the form: problems of fields named in labels go
 * beside those fields, everything else on top.
 */
export function formErrors(error: unknown, labels: Record<string, string>): FormErrors {
  const byField: Record<string, string[]> = {}
  const general: string[] = []
  if (error instanceof ApiError && Object.keys(error.fields).length > 0) {
    for (const [field, problems] of Object.entries(error.fields)) {
      if (field in labels) {
        byField[field] = problems.map((p) => `${labels[field]} ${serverMessage(p)}`)
      } else {
        general.push(...problems.map((p) => (field === 'base' ? serverMessage(p) : `${field} ${serverMessage(p)}`)))
      }
    }
  } else {
    general.push(...errorMessages(error, labels))
  }
  return { field: (name) => byField[name], general }
}
