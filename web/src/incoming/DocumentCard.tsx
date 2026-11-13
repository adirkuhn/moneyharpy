import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ListIcon } from '../layout/icons'

/** Labels beside their values, in the rows of a document that cannot change. */
export function FactRows({ rows }: { rows: [label: string, value: ReactNode][] }) {
  return (
    <dl className="incoming-card__facts">
      {rows.map(([label, value]) => (
        <div key={label} className="incoming-card__fact">
          <dt>{label}</dt>
          <dd>{value === null || value === undefined || value === '' ? <span className="incoming-card__none">—</span> : value}</dd>
        </div>
      ))}
    </dl>
  )
}

type NextProps = {
  /** How many other documents are still to process. */
  left: number
  /** Opens the next document to process; left out where Next is offered elsewhere, such as in the lines editor. */
  onNext?: () => void
}

/**
 * The foot of a document still to process: Next, to the next document to
 * process, and how many are left. Its fields save themselves, so there is
 * nothing to save here.
 */
export function NextFooter({ left, onNext }: NextProps) {
  const { t } = useTranslation()
  if (!onNext && left === 0) {
    return null
  }
  return (
    <div className="incoming-card__footer">
      {onNext && (
        <div className="incoming-card__buttons">
          <button type="button" className="btn btn--primary btn--with-icon" onClick={onNext}>
            <ListIcon kind="plus" />
            <span>{t('incoming.view.next')}</span>
          </button>
        </div>
      )}
      {left > 0 && <p className="incoming-card__left">{t('incoming.view.left', { count: left })}</p>}
    </div>
  )
}
