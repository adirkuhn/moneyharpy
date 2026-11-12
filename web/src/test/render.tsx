import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { AppRoutes } from '../App'

function CurrentPath() {
  const { pathname, search, state } = useLocation()
  return (
    <>
      <div data-testid="path">{pathname}</div>
      <div data-testid="search">{search}</div>
      <div data-testid="state">{state ? JSON.stringify(state) : ''}</div>
    </>
  )
}

/** Renders the app at a path, or at a location with the state a page passed along. */
export function renderAt(path: string | { pathname: string; search?: string; state?: unknown }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <AppRoutes />
        <CurrentPath />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}
