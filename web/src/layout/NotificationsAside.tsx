import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { BellIcon } from './icons'
import { PageIcon } from './pageIcons'

/**
 * The bell in the top navigation. Like Moneybird it opens a panel from the
 * right side of the screen instead of a small menu.
 */
export function NotificationsAside() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const bell = useRef<HTMLButtonElement>(null)
  const close = useRef<HTMLButtonElement>(null)
  const titleId = useId()
  const { pathname } = useLocation()

  useEffect(() => setOpen(false), [pathname])
  useEffect(() => {
    if (!open) {
      return
    }
    close.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false)
        bell.current?.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open])

  const hide = () => {
    setOpen(false)
    bell.current?.focus()
  }

  return (
    <>
      <button
        ref={bell}
        type="button"
        className="topnav__btn"
        aria-label={t('nav.notifications')}
        aria-expanded={open}
        aria-haspopup="dialog"
        onClick={() => setOpen(!open)}
      >
        <span className="topnav__label"><BellIcon /></span>
      </button>
      {open && (
        <>
          <div className="aside-overlay" onClick={hide} aria-hidden="true" />
          <aside className="aside" role="dialog" aria-modal="true" aria-labelledby={titleId}>
            <header className="aside__header">
              <h2 id={titleId} className="aside__title">{t('nav.notifications')}</h2>
              <button ref={close} type="button" className="icon-btn" aria-label={t('common.close')} onClick={hide}>
                <PageIcon kind="close" />
              </button>
            </header>
            <section className="aside__notifications">
              <p className="aside__empty">{t('nav.noNotifications')}</p>
            </section>
          </aside>
        </>
      )}
    </>
  )
}
