import { useAdministration } from '../administrations/queries'
import { mayOpen } from './access'
import { useAdministrationId } from './administrationId'

/**
 * What the signed-in member may do in the open administration: which pages
 * they may open, and whether they may change data. Accountants change only
 * the bookkeeping. Outside an administration, or while it loads, nothing is
 * held back; the server checks every change anyway.
 */
export function useAccess() {
  const id = useAdministrationId()
  const administration = useAdministration(id)
  const member = administration.data
  return {
    mayOpen: (path: string) => !member || mayOpen(member.permissions, path, member.user_type),
    mayChange: (bookkeeping = false) => !member || member.user_type !== 'accountant' || bookkeeping,
  }
}

/** The path of an address below the open administration, such as `contacts/new` for `/11/contacts/new`. */
export function pathBelow(to: string, administrationId: string): string | null {
  const prefix = `/${administrationId}/`
  if (!administrationId || !to.startsWith(prefix)) {
    return null
  }
  return to.slice(prefix.length).split(/[?#]/)[0] ?? ''
}
