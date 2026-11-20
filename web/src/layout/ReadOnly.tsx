import type { ReactNode } from 'react'
import { useAccess } from './useAccess'

/**
 * Shows an editor as it is to a member who may not change it, such as an
 * accountant outside the bookkeeping: every field and button inside is
 * turned off, while the content stays readable.
 */
export function ReadOnly({ children, bookkeeping = false }: { children: ReactNode; bookkeeping?: boolean }) {
  const { mayChange } = useAccess()
  if (mayChange(bookkeeping)) {
    return <>{children}</>
  }
  return <fieldset disabled className="read-only">{children}</fieldset>
}
