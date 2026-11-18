import type { Cell, Row, SheetData } from 'write-excel-file/browser'

/** The separators of numbers in the language: [group, decimal], such as [',', '.'] in English. */
function separators(locale: string): [string, string] {
  const parts = new Intl.NumberFormat(locale).formatToParts(12345.6)
  return [parts.find((p) => p.type === 'group')?.value ?? ',', parts.find((p) => p.type === 'decimal')?.value ?? '.']
}

/** An amount as a report writes it, such as 1,234.56, -€4,288.59 or €716, as a number; null for other text. */
export function amountOf(text: string, locale: string): number | null {
  const [group, decimal] = separators(locale)
  const plain = text.replace(/[\s €$£]/g, '').replace(/^[-−]/, '-')
  if (!/^-?[\d.,]+$/.test(plain) || !/\d/.test(plain)) {
    return null
  }
  const normal = plain.split(group).join('').replace(decimal, '.')
  const n = Number(normal)
  return Number.isFinite(n) ? n : null
}

/** What a cell says, without its buttons such as Collapse all. */
function textOf(el: Element) {
  const copy = el.cloneNode(true) as Element
  copy.querySelectorAll('button').forEach((b) => b.remove())
  return (copy.textContent ?? '').replace(/\s+/g, ' ').trim()
}

/**
 * The tables of a report as a sheet: a title row, then each table with its
 * name, its head in bold and its amounts as numbers, a blank row between
 * tables.
 */
export function reportSheet(title: string, tables: HTMLTableElement[], locale: string): SheetData {
  const rows: SheetData = [[{ value: title, fontWeight: 'bold' }], []]
  tables.forEach((table, i) => {
    if (i > 0) {
      rows.push([])
    }
    const name = table.getAttribute('aria-label')
    if (tables.length > 1 && name) {
      rows.push([{ value: name, fontWeight: 'bold' }])
    }
    for (const tr of Array.from(table.querySelectorAll('tr'))) {
      const head = tr.parentElement?.tagName === 'THEAD'
      const row: Row = []
      for (const cell of Array.from(tr.children)) {
        const text = textOf(cell)
        const n = head ? null : amountOf(text, locale)
        const out: Cell = n === null
          ? (text ? { value: text, ...(head ? { fontWeight: 'bold' as const } : {}) } : null)
          : { value: n, type: Number, format: '#,##0.00' }
        row.push(out)
        const span = Number(cell.getAttribute('colspan') ?? 1)
        for (let k = 1; k < span; k++) {
          row.push(null)
        }
      }
      rows.push(row)
    }
  })
  return rows
}

/** A file name from the report's name: no characters file systems refuse. */
export const fileName = (title: string) => `${title.replace(/[\\/:*?"<>|]+/g, ' ').replace(/\s+/g, ' ').trim()}.xlsx`
