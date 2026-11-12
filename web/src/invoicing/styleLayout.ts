/**
 * The blocks of a document style as Moneybird's editor arranges them: one or
 * more columns of shown fields, and a column of fields that are not shown.
 */
import type { Line } from './queries'

/** Where a field sits: a shown column and its position, or the hidden column (index columns.length). */
export type Place = { column: number; index: number }

/** The fields of a sender block, in the order the hidden column lists them. */
export const senderFields = ['company_name', 'address', 'email', 'phone', 'company_website_url', 'chamber_of_commerce', 'tax_number', 'bank_account_number', 'bank_account_bic', 'bank_account_name'] as const
/** The customer's details a document can show under its address or among its metadata. */
export const customerFields = ['customer_tax_number', 'customer_chamber_of_commerce', 'customer_id', 'customer_email', 'customer_phone'] as const
export const invoiceMetadataFields = ['invoice_id', 'invoice_date', 'due_date', 'reference', ...customerFields] as const
export const estimateMetadataFields = ['estimate_id', 'estimate_date', 'due_date', 'reference', ...customerFields] as const

/** A custom field's name in a layout, as Moneybird writes it. */
export const customFieldName = (id: string) => `custom_field_${id}`

/** The custom field a layout field stands for, if it is one. */
export function customFieldId(field: string): string | undefined {
  return /^custom_field_(\d+)$/.exec(field)?.[1]
}

/**
 * The fields of a block followed by the custom fields of the records it may
 * show: the sender's for a sender address, the customer's under its
 * address, and the document's and customer's among the metadata.
 */
export function withCustomFields(fields: readonly string[], custom: Array<{ id: string; source: string }>, sources: readonly string[]): string[] {
  return [...fields, ...custom.filter((f) => sources.includes(f.source)).map((f) => customFieldName(f.id))]
}

const blank = 'newline'

/**
 * The fields no column shows, in the order of `fields`. A blank line can be
 * used any number of times, so it stays in the hidden column when allowed.
 */
export function hiddenFields(columns: Line[][], fields: readonly string[], blankLines: boolean): string[] {
  const shown = new Set(columns.flat().map((l) => l.field))
  return [...fields.filter((f) => !shown.has(f)), ...(blankLines ? [blank] : [])]
}

/**
 * Moves a field to a place and says where it landed. A field moved to the
 * hidden column leaves the shown ones; a blank line taken from the hidden
 * column is a copy.
 */
export function moveField(columns: Line[][], fields: readonly string[], blankLines: boolean, from: Place, to: Place): { columns: Line[][]; at: Place } {
  const hiddenColumn = columns.length
  const next = columns.map((c) => [...c])
  let line: Line
  if (from.column === hiddenColumn) {
    const field = hiddenFields(columns, fields, blankLines)[from.index]
    if (field === undefined || to.column === hiddenColumn) {
      return { columns, at: from }
    }
    line = { field, bold: false, italic: false, label: false }
  } else {
    const removed = next[from.column]?.splice(from.index, 1)[0]
    if (!removed) {
      return { columns, at: from }
    }
    line = removed
  }
  if (to.column === hiddenColumn) {
    return { columns: next, at: { column: hiddenColumn, index: hiddenFields(next, fields, blankLines).indexOf(line.field) } }
  }
  const target = next[to.column] ?? []
  const index = Math.max(0, Math.min(to.index, target.length))
  target.splice(index, 0, line)
  return { columns: next, at: { column: to.column, index } }
}

/** Every line with its bold and label flags spelled out, as the API stores them. */
export function normalLines(lines: Array<Record<string, string | boolean>> | undefined, fallback: Line[]): Line[] {
  return (lines ?? fallback).map((l) => ({ field: String(l.field ?? ''), bold: l.bold === true, italic: l.italic === true, label: l.label === true }))
}
