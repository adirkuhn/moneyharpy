import { useTranslation } from 'react-i18next'
import { Link, Navigate, useParams, useSearchParams } from 'react-router-dom'
import { filterTerms, parseFilter, withTerm } from '../invoices/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { documentPath, type FilterType, hasTypedPage, useDocumentSummary } from './documents'

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

/** A document of any type by its id (/documents/:id): the page of its type, or what is known of it where there is none yet. */
export function DocumentRedirect() {
  const { t } = useTranslation()
  const { administrationId = '', documentId = '' } = useParams()
  const doc = useDocumentSummary(administrationId, documentId)
  const back = { to: `/${administrationId}/documents`, label: t('incoming.new.back') }
  if (!doc.data) {
    return doc.error ? (
      <Page title={t('incoming.pending.title')} back={back}>
        <p className="empty">{t('incoming.pending.notFound')}</p>
      </Page>
    ) : <Loading />
  }
  if (hasTypedPage(doc.data.type)) {
    return <Navigate to={documentPath(administrationId, doc.data)} replace />
  }
  return (
    <Page title={doc.data.reference || t('incoming.pending.title')} subtitle={doc.data.contact_name || undefined} back={back}>
      <p className="notice">{t('incoming.pending.text')}</p>
      <p><Link to={back.to}>{t('incoming.new.back')}</Link></p>
    </Page>
  )
}
