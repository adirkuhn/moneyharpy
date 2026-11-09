import { useQuery } from '@tanstack/react-query'
import { Navigate, useParams } from 'react-router-dom'
import { api, unwrap } from '../api/client'
import { NotFoundPage } from '../layout/NotFoundPage'

export function PermalinkPage() {
  const { id = '' } = useParams()
  const location = useQuery({
    queryKey: ['resources', id],
    queryFn: () => unwrap(api.GET('/api/resources/{id}', { params: { path: { id } } })),
  })
  if (location.isError) {
    return <NotFoundPage />
  }
  return location.data ? <Navigate to={location.data.uri} replace /> : null
}
