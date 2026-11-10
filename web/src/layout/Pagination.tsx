import { useTranslation } from 'react-i18next'
import { Link, useLocation, useSearchParams } from 'react-router-dom'
import { ArrowLeftIcon, ChevronIcon } from './icons'

/** The pages to offer around the current one, with null for a gap. */
export function pageWindow(current: number, last: number): (number | null)[] {
  const wanted = new Set([1, last, current - 1, current, current + 1].filter((p) => p >= 1 && p <= last))
  const pages = [...wanted].sort((a, b) => a - b)
  const out: (number | null)[] = []
  pages.forEach((p, i) => {
    if (i > 0 && p - (pages[i - 1] ?? p) > 1) {
      out.push(null)
    }
    out.push(p)
  })
  return out
}

/** Links to the pages of a long list, kept in the address as ?page=. */
export function Pagination({ total, perPage }: { total: number; perPage: number }) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const [search] = useSearchParams()
  const last = Math.max(1, Math.ceil(total / perPage))
  if (last === 1) {
    return null
  }
  const current = Math.min(Math.max(1, Number(search.get('page')) || 1), last)
  const to = (page: number) => {
    const next = new URLSearchParams(search)
    if (page === 1) {
      next.delete('page')
    } else {
      next.set('page', String(page))
    }
    const query = next.toString()
    return query ? `${pathname}?${query}` : pathname
  }
  return (
    <nav className="pagination" aria-label={t('common.pagination')}>
      {current > 1 ? (
        <Link className="pagination__step" to={to(current - 1)} aria-label={t('common.previousPage')}><ArrowLeftIcon /></Link>
      ) : (
        <span className="pagination__step is-disabled" aria-hidden="true"><ArrowLeftIcon /></span>
      )}
      {pageWindow(current, last).map((p, i) =>
        p === null ? (
          <span key={`gap-${i}`} className="pagination__gap" aria-hidden="true">…</span>
        ) : p === current ? (
          <span key={p} className="pagination__page is-current" aria-current="page">{p}</span>
        ) : (
          <Link key={p} className="pagination__page" to={to(p)} aria-label={t('common.page', { n: p })}>{p}</Link>
        ),
      )}
      {current < last ? (
        <Link className="pagination__step" to={to(current + 1)} aria-label={t('common.nextPage')}><ChevronIcon /></Link>
      ) : (
        <span className="pagination__step is-disabled" aria-hidden="true"><ChevronIcon /></span>
      )}
    </nav>
  )
}
