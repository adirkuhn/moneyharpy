import { type ReactNode, useId } from 'react'
import { CaretIcon } from '../layout/icons'

type Props = {
  title: string
  icon: ReactNode
  open: boolean
  onToggle: () => void
  /** A part that cannot be changed here; `tip` says why. */
  disabled?: boolean
  tip?: string
  children?: ReactNode
}

/** One part of Moneybird's style editor: a header that opens its settings below it. */
export function AccordionItem({ title, icon, open, onToggle, disabled = false, tip, children }: Props) {
  const id = useId()
  return (
    <div className={`style-accordion__item${open ? ' is-open' : ''}${disabled ? ' is-disabled' : ''}`} title={tip}>
      <h3 className="style-accordion__heading">
        <button type="button" id={`${id}-button`} className="style-accordion__header" aria-expanded={open && !disabled} aria-controls={`${id}-section`} disabled={disabled} onClick={onToggle}>
          <span className="style-accordion__icon" aria-hidden="true">{icon}</span>
          <strong className="style-accordion__title">{title}</strong>
          <span className="style-accordion__caret" aria-hidden="true"><CaretIcon /></span>
        </button>
      </h3>
      <section id={`${id}-section`} className="style-accordion__body" role="region" aria-labelledby={`${id}-button`} hidden={!open || disabled}>
        {children}
      </section>
    </div>
  )
}

/** Small line icons for the parts of the style editor, drawn for MoneyHarpy. */
export function PartIcon({ kind }: { kind: 'image' | 'font' | 'layout' | 'card' | 'list' | 'code' | 'cog' | 'mail' }) {
  const shapes = {
    image: <><rect x="3.5" y="5" width="17" height="14" rx="2" /><circle cx="9" cy="10" r="1.5" /><path d="m4 17 5-4.5 3.5 3 3-2.5 4.5 4" /></>,
    font: <path d="M5 19 10.500 5h1L17 19M7.300 14h7.400M15 19h4" />,
    layout: <><rect x="3.5" y="4" width="17" height="16" rx="2" /><path d="M3.5 9h17M10 9v11" /></>,
    card: <><rect x="3.5" y="5" width="17" height="14" rx="2" /><circle cx="9" cy="11" r="2" /><path d="M6 16c.6-1.300 1.700-2 3-2s2.400.7 3 2M14.500 10h3.500M14.500 13.500h3.500" /></>,
    list: <path d="M9 7h11M9 12h11M9 17h11M4.500 7h.01M4.500 12h.01M4.500 17h.01" />,
    code: <path d="m8.500 8-4 4 4 4M15.500 8l4 4-4 4M13.500 5.500l-3 13" />,
    cog: <><circle cx="12" cy="12" r="3" /><path d="M12 3.500v2.200M12 18.300v2.200M3.500 12h2.200M18.300 12h2.200M6 6l1.600 1.600M16.400 16.400 18 18M6 18l1.600-1.600M16.400 7.600 18 6" /></>,
    mail: <><rect x="3.5" y="5.500" width="17" height="13" rx="2" /><path d="m4 7 8 6 8-6" /></>,
  }
  return (
    <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {shapes[kind]}
    </svg>
  )
}
