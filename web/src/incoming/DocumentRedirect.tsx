import { Navigate, useParams, useSearchParams } from 'react-router-dom'
import { filterTerms, parseFilter, withTerm } from '../invoices/queries'
import { documentPath, type FilterType } from './documents'

/** The old list of one type (/documents/purchase_invoices and the like), now Incoming filtered on that type. */
export function TypeListRedirect({ type }: { type: FilterType | null }) {
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const filter = parseFilter(params.get('filter'))
  // Uploads have no type; they are the new documents.
  const next = type ? { ...filter, rest: withTerm(filter.rest, 'type', type) } : { ...filter, state: 'new', period: '' }
  const query = new URLSearchParams({ filter: filterTerms(next) })
  return <Navigate to={`/${administrationId}/documents?${query}`} replace />
}

/** The old page of a purchase invoice or receipt (/documents/receipts/:id), now the one page of every document. */
export function TypedDocumentRedirect() {
  const { administrationId = '', documentId = '' } = useParams()
  return <Navigate to={documentPath(administrationId, { id: documentId })} replace />
}
