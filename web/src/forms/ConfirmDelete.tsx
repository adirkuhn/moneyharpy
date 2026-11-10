import { useState } from 'react'

type Props = {
  /** Names the first button, such as "Delete Export". */
  label: string
  /** Names the second button, such as "Confirm delete Export". */
  confirmLabel: string
  /** The visible text of each step; icon-only rows can pass short text. */
  text?: string
  confirmText?: string
  disabled?: boolean
  onConfirm: () => void
}

/**
 * A delete button that asks once more before acting, since deleting cannot be
 * undone.
 */
export function ConfirmDelete({ label, confirmLabel, text = label, confirmText = confirmLabel, disabled, onConfirm }: Props) {
  const [confirming, setConfirming] = useState(false)
  if (confirming) {
    return (
      <button type="button" className="btn btn--danger btn--small" aria-label={confirmLabel} disabled={disabled} onClick={onConfirm} onBlur={() => setConfirming(false)} autoFocus>
        {confirmText}
      </button>
    )
  }
  return (
    <button type="button" className="btn btn--secondary btn--small" aria-label={label} disabled={disabled} onClick={() => setConfirming(true)}>
      {text}
    </button>
  )
}
