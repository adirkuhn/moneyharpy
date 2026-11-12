import { useId } from 'react'
import { useTranslation } from 'react-i18next'

type Props = {
  /** Editing or deleting the sent invoice. */
  kind: 'edit' | 'delete'
  /** A paid invoice is refunded rather than changed. */
  paid: boolean
  busy?: boolean
  onCredit: () => void
  onContinue: () => void
  onCancel: () => void
}

/**
 * The warning before a sent invoice is edited or deleted. It may, as at
 * Moneybird, but the regular way is to cancel or refund it with a credit
 * note, so that comes first.
 */
export function SentWarning({ kind, paid, busy, onCredit, onContinue, onCancel }: Props) {
  const { t } = useTranslation()
  const id = useId()
  const deleting = kind === 'delete'
  const title = deleting ? t('invoices.view.sentWarning.deleteTitle') : paid ? t('invoices.view.sentWarning.paidTitle') : t('invoices.view.sentWarning.title')
  return (
    <div className={`confirm-panel${deleting ? '' : ' confirm-panel--calm'}`} role="alertdialog" aria-labelledby={id} aria-describedby={`${id}-text`}>
      <div>
        <p id={id} className="confirm-panel__message">{title}</p>
        <div id={`${id}-text`}>
          <p className="confirm-panel__text">{deleting ? t('invoices.view.sentWarning.deleteText') : t('invoices.view.sentWarning.text')}</p>
          <p className="confirm-panel__text">
            {t('invoices.view.sentWarning.regular')}
            {paid && <> {t('invoices.view.sentWarning.refund')}</>}
          </p>
        </div>
      </div>
      <div className="confirm-panel__buttons">
        <button type="button" className="btn btn--primary" disabled={busy} onClick={onCredit} autoFocus>{t('invoices.view.duplicateCredit')}</button>
        <button type="button" className={`btn ${deleting ? 'btn--danger' : 'btn--secondary'}`} disabled={busy} onClick={onContinue}>
          {deleting ? t('invoices.view.sentWarning.deleteAnyway') : t('invoices.view.sentWarning.continue')}
        </button>
        <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
      </div>
    </div>
  )
}
