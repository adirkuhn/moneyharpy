import type { FormEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ListIcon } from '../layout/icons'

/** What the footer of a document's form needs: how to save, and what is left to process. */
export type Saving = {
  pending: boolean
  /** Saves; with next, the next document to process opens after. */
  save: (next: boolean) => void
  /** How many other documents are still to process. */
  left: number
  /** Where Cancel leads: Incoming. */
  cancelTo: string
}

type FormProps = { label: string; saving: Saving; children: ReactNode }

/**
 * The card at the right of a document being edited, after Moneybird's
 * document form: its rows, then Save and Next joined, Cancel, and how many
 * documents are left to process.
 */
export function DocumentForm({ label, saving, children }: FormProps) {
  const { t } = useTranslation()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    saving.save(false)
  }
  return (
    <form className="incoming-card" aria-label={label} onSubmit={submit} noValidate>
      {children}
      <div className="incoming-card__footer">
        <div className="incoming-card__buttons">
          <div className="btn-group">
            <button type="submit" className="btn btn--primary btn--with-icon" disabled={saving.pending}>
              <CheckIcon />
              <span>{t('incoming.view.save')}</span>
            </button>
            <button type="button" className="btn btn--primary btn--with-icon" disabled={saving.pending} onClick={() => saving.save(true)}>
              <ListIcon kind="plus" />
              <span>{t('incoming.view.next')}</span>
            </button>
          </div>
          <Link className="btn btn--secondary" to={saving.cancelTo}>{t('common.cancel')}</Link>
        </div>
        {saving.left > 0 && <p className="incoming-card__left">{t('incoming.view.left', { count: saving.left })}</p>}
      </div>
    </form>
  )
}

type ViewProps = { label: string; onEdit?: () => void; children: ReactNode }

/** The same card for a saved document: its values, and Edit where it can still change. */
export function DocumentFacts({ label, onEdit, children }: ViewProps) {
  const { t } = useTranslation()
  return (
    <section className="incoming-card" aria-label={label}>
      {children}
      {onEdit && (
        <div className="incoming-card__footer">
          <div className="incoming-card__buttons">
            <button type="button" className="btn btn--secondary btn--with-icon" onClick={onEdit}>
              <ListIcon kind="edit" />
              <span>{t('incoming.view.edit')}</span>
            </button>
          </div>
        </div>
      )}
    </section>
  )
}

/** Labels beside their values, in the rows of the form. */
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

function CheckIcon() {
  return (
    <svg viewBox="0 0 24 24" width="1em" height="1em" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="m5 12.5 4.5 4.5L19 7.5" />
    </svg>
  )
}
