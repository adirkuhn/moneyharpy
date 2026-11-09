import { Link } from 'react-router-dom'
import { Page } from './Page'

export function NotFoundPage() {
  return (
    <Page title="Page not found">
      <p className="empty">
        This page does not exist or you have no access to it. <Link to="/administrations">Go to your administrations</Link>
      </p>
    </Page>
  )
}
