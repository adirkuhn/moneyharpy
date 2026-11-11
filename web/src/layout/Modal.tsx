import { type ReactNode, useEffect, useId, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { PageIcon } from './pageIcons'
import './modal.css'

const focusable = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

type Props = {
  title: string
  /** A line under the title, such as what to do first. */
  subtitle?: string
  onClose: () => void
  children: ReactNode
  /** The buttons along the bottom. */
  footer?: ReactNode
  /** An alert dialog asks to confirm something and has no close button. */
  alert?: boolean
  className?: string
}

/**
 * A popup over the page, after Moneybird's modals: a title with a close
 * button, a body and buttons at the bottom. Focus moves into it and stays
 * there; Escape closes it and gives focus back to where it came from.
 */
export function Modal({ title, subtitle, onClose, children, footer, alert = false, className = '' }: Props) {
  const { t } = useTranslation()
  const id = useId()
  const box = useRef<HTMLDivElement>(null)
  const close = useRef(onClose)
  close.current = onClose

  useEffect(() => {
    const before = document.activeElement as HTMLElement | null
    if (!box.current?.contains(before)) {
      const first = box.current?.querySelector<HTMLElement>('[data-autofocus]') ?? box.current?.querySelector<HTMLElement>(focusable.split(', ').map((f) => `.modal__body ${f}`).join(', '))
      ;(first ?? box.current)?.focus()
    }
    return () => {
      if (before?.isConnected) {
        before.focus()
      }
    }
  }, [])

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      e.preventDefault()
      close.current()
      return
    }
    if (e.key !== 'Tab' || !box.current) {
      return
    }
    const all = [...box.current.querySelectorAll<HTMLElement>(focusable)]
    const first = all[0]
    const last = all[all.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last?.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first?.focus()
    }
  }

  return (
    <div className={`modal${alert ? ' modal--alert' : ''}`}>
      <div className="modal__overlay" aria-hidden="true" onClick={alert ? undefined : onClose} />
      <div
        ref={box}
        className={`modal__box ${className}`}
        role={alert ? 'alertdialog' : 'dialog'}
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        aria-describedby={subtitle ? `${id}-subtitle` : undefined}
        tabIndex={-1}
        onKeyDown={onKeyDown}
      >
        <header className="modal__header">
          <div>
            <h2 id={`${id}-title`} className="modal__title">{title}</h2>
            {subtitle && <p id={`${id}-subtitle`} className="modal__subtitle">{subtitle}</p>}
          </div>
          {!alert && (
            <button type="button" className="icon-btn modal__close" aria-label={t('common.close')} title={t('common.close')} onClick={onClose}>
              <PageIcon kind="close" />
            </button>
          )}
        </header>
        {children && <div className="modal__body">{children}</div>}
        {footer && <footer className="modal__footer">{footer}</footer>}
      </div>
    </div>
  )
}

type ConfirmProps = {
  title: string
  description: string
  confirmLabel: string
  onConfirm: () => void
  onCancel: () => void
}

/** Asks once more before something that changes how the invoice behaves, such as unlocking its number. */
export function ConfirmDialog({ title, description, confirmLabel, onConfirm, onCancel }: ConfirmProps) {
  const { t } = useTranslation()
  return (
    <Modal
      alert
      title={title}
      subtitle={description}
      onClose={onCancel}
      footer={
        <>
          <button type="button" className="btn btn--primary" data-autofocus onClick={onConfirm}>{confirmLabel}</button>
          <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
        </>
      }
    >
      {null}
    </Modal>
  )
}
