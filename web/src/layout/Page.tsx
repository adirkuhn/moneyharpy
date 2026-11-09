import type { ReactNode } from 'react'

export function Page({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <>
      <header className="actionbar">
        <h1>{title}</h1>
        {actions && <div className="actionbar__actions">{actions}</div>}
      </header>
      <div className="content">{children}</div>
    </>
  )
}
