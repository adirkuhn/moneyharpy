import type { MouseEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { ListIcon } from '../layout/icons'
import { Pagination } from '../layout/Pagination'

export type Column<T> = {
  key: string
  header: ReactNode
  cell: (row: T) => ReactNode
  /** The cell that links to the document; its text names the row. */
  link?: boolean
  /** Amounts align right, dates and states keep to one line, icons stay narrow, extras give way on small screens. */
  kind?: 'amount' | 'date' | 'state' | 'icons' | 'extra'
  /** A name for a column without a header, such as one of icons. */
  label?: string
}

type Selection<T> = {
  selected: Set<string>
  onChange: (selected: Set<string>) => void
  selectLabel: (row: T) => string
  allLabel: string
}

type Props<T> = {
  label: string
  rows: T[]
  rowKey: (row: T) => string
  /** Where a row leads; a row without a page of its own is not a link. */
  href: (row: T) => string | undefined
  columns: Column<T>[]
  /** Checkboxes for the actions on several rows at once. */
  selection?: Selection<T>
  total: number
  perPage: number
}

const interactive = 'a, button, input, label, select, textarea'

/** A page of documents, after Moneybird's lists: every row opens its document, rows can be ticked, and long lists are paged. */
export function DocumentTable<T>({ label, rows, rowKey, href, columns, selection, total, perPage }: Props<T>) {
  const navigate = useNavigate()
  const keys = rows.map(rowKey)
  const allChosen = selection !== undefined && keys.length > 0 && keys.every((k) => selection.selected.has(k))

  // The link in the row serves the keyboard and new tabs; this makes the rest of the row a target too.
  const open = (e: MouseEvent, to: string | undefined) => {
    if (to === undefined || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || (e.target as Element).closest(interactive)) {
      return
    }
    if (window.getSelection()?.toString()) {
      return
    }
    navigate(to)
  }
  const toggle = (key: string) => {
    if (!selection) {
      return
    }
    const next = new Set(selection.selected)
    if (!next.delete(key)) {
      next.add(key)
    }
    selection.onChange(next)
  }
  const cellClass = (c: Column<T>) => (c.kind ? `document-table__${c.kind}` : undefined)

  return (
    <>
      <div className="document-table__scroll">
        <table className="data-table document-table" aria-label={label}>
          <thead>
            <tr>
              {selection && (
                <th scope="col" className="document-table__check">
                  <input type="checkbox" aria-label={selection.allLabel} checked={allChosen} onChange={() => selection.onChange(allChosen ? new Set() : new Set(keys))} />
                </th>
              )}
              {columns.map((c) => (
                <th key={c.key} scope="col" className={cellClass(c)} aria-label={c.label}>{c.header}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const key = rowKey(row)
              const to = href(row)
              return (
                <tr key={key} className={`document-table__row${to === undefined ? ' is-plain' : ''}${selection?.selected.has(key) ? ' is-selected' : ''}`} onClick={(e) => open(e, to)}>
                  {selection && (
                    <td className="document-table__check">
                      <input type="checkbox" aria-label={selection.selectLabel(row)} checked={selection.selected.has(key)} onChange={() => toggle(key)} />
                    </td>
                  )}
                  {columns.map((c) => (
                    <td key={c.key} className={cellClass(c)}>
                      {c.link && to !== undefined ? <Link className="document-table__link" to={to}>{c.cell(row)}</Link> : c.cell(row)}
                    </td>
                  ))}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <Pagination total={total} perPage={perPage} />
    </>
  )
}

/** A paperclip for a document with attachments, for the icons column. */
export function AttachmentIcon({ count }: { count: number }) {
  const { t } = useTranslation()
  if (count === 0) {
    return null
  }
  return (
    <span className="document-table__icon" role="img" aria-label={t('list.hasAttachments')} title={t('list.hasAttachments')}>
      <ListIcon kind="paperclip" />
    </span>
  )
}
