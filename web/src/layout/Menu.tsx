import { type ReactNode, useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { CaretIcon } from './icons'

type MenuProps = {
  label: ReactNode
  ariaLabel?: string
  caret?: boolean
  align?: 'left' | 'right'
  /** Styles the button; the top-navigation look by default. */
  buttonClassName?: string
  children: ReactNode
}

/** A top-navigation button that opens a panel. */
export function Menu({ label, ariaLabel, caret = true, align = 'left', buttonClassName = 'topnav__btn', children }: MenuProps) {
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const { pathname } = useLocation()

  useEffect(() => setOpen(false), [pathname])
  useEffect(() => {
    if (!open) {
      return
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    const onClick = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('keydown', onKey)
    document.addEventListener('click', onClick)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('click', onClick)
    }
  }, [open])

  return (
    <div className="menu" ref={root}>
      <button type="button" className={buttonClassName} aria-label={ariaLabel} aria-expanded={open} aria-haspopup="true" onClick={() => setOpen(!open)}>
        <span className="topnav__label">{label}</span>
        {caret && <CaretIcon />}
      </button>
      {open && <div className={`menu__panel menu__panel--${align}`}>{children}</div>}
    </div>
  )
}
