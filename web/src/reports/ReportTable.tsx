import { createContext, Fragment, type ReactNode, useContext, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import type { LedgerAccount } from '../bookkeeping/queries'
import { CaretIcon } from '../layout/icons'
import type { Order } from './ReportFrame'

/** An amount as Moneybird's reports write it: two decimals, no currency, such as 19,502.05. */
export function useAmount() {
  const { i18n } = useTranslation()
  return useMemo(() => {
    const format = new Intl.NumberFormat(i18n.language, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
    return (n: number | string) => format.format(Number(n) || 0)
  }, [i18n.language])
}

/** A column of amounts: its head and, for amounts that can be looked into, the period it covers. */
export type Column = { key: string; label: ReactNode; period?: string }

/** The card a report sits in, with Moneybird's note under it. */
export function ReportCard({ children, note }: { children: ReactNode; note?: string }) {
  return (
    <section className="report-card">
      {children}
      {note && <p className="report-card__note">{note}</p>}
    </section>
  )
}

/** A report's table: links to change how it shows in the corner, a column per part of the period. */
export function ReportTable({ label, columns, tools, children, className = '' }: { label: string; columns: Column[]; tools?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <table className={`report-table ${className}`} aria-label={label}>
      <thead>
        <tr>
          <th scope="col" className="report-table__tools">{tools}</th>
          {columns.map((c) => <th key={c.key} scope="col" className="report-table__amount">{c.label}</th>)}
        </tr>
      </thead>
      {children}
    </table>
  )
}

/** A titled part of a report, such as Revenue, with its rows. */
export function Section({ title, span, children }: { title?: string; span: number; children: ReactNode }) {
  return (
    <tbody className="report-table__section">
      {title && (
        <tr className="report-table__heading">
          <th scope="rowgroup" colSpan={span}>{title}</th>
        </tr>
      )}
      {children}
    </tbody>
  )
}

/** The amounts of a row; with a link, those that are not zero lead to what makes them up. */
export function Amounts({ values, link }: { values: number[]; link?: (i: number) => string | undefined }) {
  const amount = useAmount()
  return (
    <>
      {values.map((v, i) => {
        const to = v !== 0 ? link?.(i) : undefined
        return (
          <td key={i} className="report-table__amount">
            {to ? <Link to={to}>{amount(v)}</Link> : amount(v)}
          </td>
        )
      })}
    </>
  )
}

/** A row adding up what is above it, such as Total revenue or Gross profit. */
export function TotalRow({ label, values, grand = false }: { label: ReactNode; values: number[]; grand?: boolean }) {
  return (
    <tr className={`report-table__total${grand ? ' is-grand' : ''}`}>
      <th scope="row">{label}</th>
      <Amounts values={values} />
    </tr>
  )
}

/** A category with its amounts, those of its sub-categories included, and its sub-categories. */
export type TreeNode = { key: string; id: string; name: string; code?: string; values: number[]; children: TreeNode[] }

const add = (a: number[], b: number[]) => a.map((v, i) => v + (b[i] ?? 0))

function comparer(order: Order) {
  const name = (a: LedgerAccount, b: LedgerAccount) => (a.name ?? '').localeCompare(b.name ?? '')
  const by = (key: (a: LedgerAccount) => string) => (a: LedgerAccount, b: LedgerAccount) =>
    key(a).localeCompare(key(b), undefined, { numeric: true }) || name(a, b)
  if (order === 'account_id') {
    return by((a) => a.account_id ?? '')
  }
  if (order === 'rgs') {
    return by((a) => a.taxonomy_item?.code ?? '')
  }
  return name
}

/**
 * The categories with amounts as Moneybird nests them under their top
 * categories, a top category adding up its own amounts and those below it.
 * Amounts booked on a category that has sub-categories show as its own
 * "General" sub-category.
 */
export function categoryTree(values: Map<string, number[]>, accounts: LedgerAccount[], width: number, order: Order, general: (name: string) => string): TreeNode[] {
  const index = new Map(accounts.map((a) => [a.id ?? '', a]))
  const included = new Set<string>()
  for (const id of values.keys()) {
    let at: string | null | undefined = id
    for (let seen = 0; at && !included.has(at) && seen < 32; seen++) {
      included.add(at)
      const parent: string | number | null | undefined = index.get(at)?.parent_id
      at = parent == null ? null : String(parent)
      if (at && !index.has(at)) {
        break
      }
    }
  }
  const sort = comparer(order)
  const list = (parent: string | null) => [...included]
    .map((id) => index.get(id) ?? ({ id, name: id } as LedgerAccount))
    .filter((a) => {
      const p = a.parent_id != null && included.has(String(a.parent_id)) ? String(a.parent_id) : null
      return p === parent
    })
    .sort(sort)
  const node = (a: LedgerAccount): TreeNode => {
    const id = a.id ?? ''
    const own = values.get(id) ?? Array<number>(width).fill(0)
    const children = list(id).map(node)
    const base = { id, name: a.name ?? id, code: a.account_id ?? undefined }
    if (children.length === 0) {
      return { ...base, key: id, values: own, children }
    }
    if (own.some((v) => v !== 0)) {
      children.unshift({ ...base, key: `${id}:own`, name: general(base.name), values: own, children: [] })
    }
    return { ...base, key: id, values: children.reduce((sum, c) => add(sum, c.values), Array<number>(width).fill(0)), children }
  }
  return list(null).map(node)
}

type Collapsing = { collapsed: Set<string>; toggle: (key: string) => void }
const CollapseContext = createContext<Collapsing>({ collapsed: new Set(), toggle: () => {} })

/** Which categories are folded up, and Moneybird's Collapse all and Expand all. */
export function useCollapse(nodes: TreeNode[]) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const parents = useMemo(() => {
    const out: string[] = []
    const walk = (ns: TreeNode[]) => ns.forEach((n) => {
      if (n.children.length > 0) {
        out.push(n.key)
        walk(n.children)
      }
    })
    walk(nodes)
    return out
  }, [nodes])
  const all = parents.length > 0 && parents.every((k) => collapsed.has(k))
  return {
    context: {
      collapsed,
      toggle: (key: string) => setCollapsed((c) => {
        const next = new Set(c)
        if (!next.delete(key)) {
          next.add(key)
        }
        return next
      }),
    },
    allCollapsed: all,
    toggleAll: () => setCollapsed(all ? new Set() : new Set(parents)),
    any: parents.length > 0,
  }
}

export function CollapseProvider({ value, children }: { value: Collapsing; children: ReactNode }) {
  return <CollapseContext.Provider value={value}>{children}</CollapseContext.Provider>
}

/** Moneybird's Collapse all link in the corner of a report. */
export function CollapseAll({ collapse }: { collapse: ReturnType<typeof useCollapse> }) {
  const { t } = useTranslation()
  if (!collapse.any) {
    return null
  }
  return (
    <button type="button" className="report-tool" onClick={collapse.toggleAll}>
      <CaretIcon />
      <span>{collapse.allCollapsed ? t('reports.expandAll') : t('reports.collapseAll')}</span>
    </button>
  )
}

/** The rows of categories, each linking to its bookings, the top ones with a button to fold up those below. */
export function CategoryRows({ nodes, link, depth = 0, foldable = true }: { nodes: TreeNode[]; link: (id: string, column?: number) => string | undefined; depth?: number; foldable?: boolean }) {
  const { t } = useTranslation()
  const { collapsed, toggle } = useContext(CollapseContext)
  return (
    <>
      {nodes.map((n) => {
        const open = !collapsed.has(n.key)
        const to = link(n.id)
        return (
          <Fragment key={n.key}>
            <tr className={`report-table__category is-depth-${Math.min(depth, 3)}`}>
              <th scope="row">
                <span className="report-table__name">
                  {foldable && n.children.length > 0 && (
                    <button
                      type="button"
                      className="report-table__fold"
                      aria-expanded={open}
                      aria-label={t(open ? 'reports.collapse' : 'reports.expand', { name: n.name })}
                      onClick={() => toggle(n.key)}
                    >
                      <CaretIcon />
                    </button>
                  )}
                  {to ? <Link to={to}>{n.name}</Link> : <span>{n.name}</span>}
                  {n.code && <>{' '}<span className="report-table__code">{n.code}</span></>}
                </span>
              </th>
              <Amounts values={n.values} link={n.children.length > 0 ? undefined : (i) => link(n.id, i)} />
            </tr>
            {open && n.children.length > 0 && <CategoryRows nodes={n.children} link={link} depth={depth + 1} foldable={foldable} />}
          </Fragment>
        )
      })}
    </>
  )
}
