import { Link, type LinkProps } from 'react-router-dom'
import { useAdministrationId } from './administrationId'
import { pathBelow, useAccess } from './useAccess'

/**
 * A link styled as an action, such as Add or Edit. It is left out when the
 * member may not open the page it leads to, so nobody is offered a page
 * that would only say No access.
 */
export function ActionLink(props: LinkProps & { to: string }) {
  const access = useAccess()
  const path = pathBelow(props.to, useAdministrationId())
  if (path !== null && !access.mayOpen(path)) {
    return null
  }
  return <Link {...props} />
}
