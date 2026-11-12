import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeftIcon } from './icons'

type Props = {
  title: string
  subtitle?: string
  /** Where the page's back button leads, as on Moneybird's edit pages. */
  back?: { to: string; label: string }
  /** What comes before the title, such as the filter button of a list. */
  lead?: ReactNode
  actions?: ReactNode
  /** Lists use the width of the screen. */
  wide?: boolean
  /** The title is for screen readers only, as on Moneybird's document pages, where the document says what it is. */
  hideTitle?: boolean
  children: ReactNode
}

/**
 * A page with its action bar. Beside a filter button the title is for screen
 * readers only, as on Moneybird's lists: the button already says what is shown.
 */
export function Page({ title, subtitle, back, lead, actions, wide = false, hideTitle = false, children }: Props) {
  const titleHidden = hideTitle || (lead !== undefined && back === undefined)
  return (
    <>
      <header className={`actionbar${wide ? ' actionbar--wide' : ''}`}>
        <div className="actionbar__heading">
          {back && (
            <Link className="actionbar__back" to={back.to} aria-label={back.label} title={back.label}>
              <ArrowLeftIcon />
            </Link>
          )}
          {lead}
          <div>
            <h1 className={titleHidden ? 'sr-only' : undefined}>{title}</h1>
            {subtitle && <p className="actionbar__subtitle">{subtitle}</p>}
          </div>
        </div>
        {actions && <div className="actionbar__actions">{actions}</div>}
      </header>
      <div className={`content${wide ? ' content--wide' : ''}`}>{children}</div>
    </>
  )
}
