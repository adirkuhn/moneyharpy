import { type ReactNode, useEffect, useId, useRef, useState } from 'react'
import { ListIcon } from '../layout/icons'

type Props = {
  /** Names the button when what it shows does not, such as an empty text. */
  label?: string
  className?: string
  /** What the document shows while it is not being edited. */
  view: ReactNode
  /** The control that edits it, given a function that closes it again. */
  editor: (close: () => void) => ReactNode
  errors?: string[]
}

/**
 * A part of a document that turns into its control when clicked, as on
 * Moneybird's invoice: Escape or Enter close it, and so does leaving it.
 */
export function InPlace({ label, className = '', view, editor, errors }: Props) {
  const [editing, setEditing] = useState(false)
  const button = useRef<HTMLButtonElement>(null)
  const refocus = useRef(false)
  const messageId = useId()
  const invalid = errors !== undefined && errors.length > 0

  useEffect(() => {
    if (!editing && refocus.current) {
      refocus.current = false
      button.current?.focus()
    }
  }, [editing])

  const close = () => {
    refocus.current = true
    setEditing(false)
  }
  const message = invalid && <p id={messageId} className="form-input__message form-input__message--error">{errors.join(' ')}</p>

  if (editing) {
    return (
      <div
        className={`in-place in-place--editing ${className}`}
        onBlur={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
            setEditing(false)
          }
        }}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault()
            close()
          }
        }}
      >
        {editor(close)}
        {message}
      </div>
    )
  }
  return (
    <div className={`in-place ${className}`}>
      <button
        ref={button}
        type="button"
        className={`in-place__button${invalid ? ' is-invalid' : ''}`}
        aria-label={label}
        aria-invalid={invalid || undefined}
        aria-describedby={invalid ? messageId : undefined}
        onClick={() => setEditing(true)}
      >
        {view}
        <span className="in-place__indicator"><ListIcon kind="edit" /></span>
      </button>
      {message}
    </div>
  )
}

/** Closes an in-place editor on Enter, without sending the form it is in. */
export const closeOnEnter = (close: () => void) => (e: React.KeyboardEvent) => {
  if (e.key === 'Enter') {
    e.preventDefault()
    close()
  }
}
