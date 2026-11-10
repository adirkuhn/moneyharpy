import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

type Props = {
  title: string
  subtitle?: string
  /** Where the page's back button leads, as on Moneybird's edit pages. */
  back?: { to: string; label: string }
  actions?: ReactNode
  children: ReactNode
}

export function Page({ title, subtitle, back, actions, children }: Props) {
  return (
    <>
      <header className="actionbar">
        <div className="actionbar__heading">
          {back && <Link className="actionbar__back" to={back.to}>{back.label}</Link>}
          <div>
            <h1>{title}</h1>
            {subtitle && <p className="actionbar__subtitle">{subtitle}</p>}
          </div>
        </div>
        {actions && <div className="actionbar__actions">{actions}</div>}
      </header>
      <div className="content">{children}</div>
    </>
  )
}
