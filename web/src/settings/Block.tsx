import { type ReactNode, useId } from 'react'

type BlockProps = { id: string; title: string; subtitle: string; action?: ReactNode; children: ReactNode }

/** A titled part of a settings page that the section menu can link to. */
export function Block({ id, title, subtitle, action, children }: BlockProps) {
  const headingId = useId()
  return (
    <section id={id} className="settings-block" aria-labelledby={headingId}>
      <header className="settings-block__header">
        <div>
          <h2 id={headingId}>{title}</h2>
          <p className="hint">{subtitle}</p>
        </div>
        {action}
      </header>
      {children}
    </section>
  )
}
