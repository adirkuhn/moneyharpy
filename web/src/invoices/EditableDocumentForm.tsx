import { type KeyboardEvent, type ReactNode, useLayoutEffect, useRef } from 'react'

type Props = {
  label: string
  className?: string
  children: ReactNode
  onKeyDown?: (e: KeyboardEvent<HTMLElement>) => void
}

/** The room kept under a card on the document, as on Moneybird, where the document goes on a little under it. */
const roomBelow = 40

/**
 * A card that edits a part of the document, after Moneybird's
 * editable-document-form: laid over the document at the place of that part.
 * The document grows when the card is longer than what is left of it, so the
 * card never hangs over what comes after the document.
 */
export function EditableDocumentForm({ label, className = '', children, onKeyDown }: Props) {
  const card = useRef<HTMLElement>(null)
  useLayoutEffect(() => {
    const el = card.current
    const sheet = el?.offsetParent
    if (!el || !(sheet instanceof HTMLElement) || getComputedStyle(el).position !== 'absolute') {
      return
    }
    const fit = () => {
      sheet.style.minHeight = `${el.offsetTop + el.offsetHeight + roomBelow}px`
    }
    fit()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(fit)
    observer?.observe(el)
    return () => {
      observer?.disconnect()
      sheet.style.minHeight = ''
    }
  }, [])
  return (
    <section ref={card} className={`editable-document-form ${className}`} aria-label={label} onKeyDown={onKeyDown}>
      {children}
    </section>
  )
}
