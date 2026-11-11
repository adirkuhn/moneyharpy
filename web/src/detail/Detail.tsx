import { type ReactNode, useId } from 'react'
import { useTranslation } from 'react-i18next'

type BlockProps = {
  title: string
  subtitle?: string
  /** Actions of this block only, on the right of its title. */
  actions?: ReactNode
  id?: string
  children: ReactNode
}

/**
 * A part of a document's page, after Moneybird's blocks: a title with its own
 * actions, and the content under it.
 */
export function DetailBlock({ title, subtitle, actions, id, children }: BlockProps) {
  const heading = useId()
  return (
    <section id={id} className="detail-block" aria-labelledby={heading}>
      <header className="detail-block__header">
        <div>
          <h2 id={heading} className="detail-block__title">{title}</h2>
          {subtitle && <p className="detail-block__subtitle">{subtitle}</p>}
        </div>
        {actions && <div className="detail-block__actions">{actions}</div>}
      </header>
      {children}
    </section>
  )
}

/** A label and what it says; rows without a value are left out. */
export type DetailRow = [label: string, value: ReactNode]

/** The facts of a document as labels beside their values, on one sheet. */
export function DetailList({ rows, header }: { rows: DetailRow[]; header?: ReactNode }) {
  return (
    <div className="detail-list">
      {header && <div className="detail-list__header">{header}</div>}
      <dl className="detail-list__rows">
        {rows.filter(([, value]) => value !== undefined && value !== null && value !== false && value !== '').map(([label, value]) => (
          <div key={label} className="detail-list__row">
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

type ConfirmProps = {
  /** What will happen, such as "Delete this invoice? This cannot be undone." */
  message: string
  /** What follows from it, such as "You cannot undo this.", under the question. */
  description?: string
  confirmLabel: string
  /** Something that can be undone, such as archiving, is asked without the colours of danger. */
  danger?: boolean
  disabled?: boolean
  onConfirm: () => void
  onCancel: () => void
}

/** Asks once more before something that cannot be undone, chosen from the ⋯ menu. */
export function ConfirmPanel({ message, description, confirmLabel, danger = true, disabled, onConfirm, onCancel }: ConfirmProps) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div className={`confirm-panel${danger ? '' : ' confirm-panel--calm'}`} role="alertdialog" aria-labelledby={id} aria-describedby={description ? `${id}-text` : undefined}>
      <div>
        <p id={id} className="confirm-panel__message">{message}</p>
        {description && <p id={`${id}-text`} className="confirm-panel__text">{description}</p>}
      </div>
      <div className="confirm-panel__buttons">
        <button type="button" className={`btn ${danger ? 'btn--danger' : 'btn--primary'}`} disabled={disabled} onClick={onConfirm} autoFocus>{confirmLabel}</button>
        <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
      </div>
    </div>
  )
}
