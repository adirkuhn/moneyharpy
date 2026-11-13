import type { components } from '../api/schema'
import type { Confidence } from '../forms/Remark'
import type { LineField, LineRemarks } from '../invoices/DetailsEditor'

export type Recognition = components['schemas']['Recognition']

/** The fields of a document Scan and Recognise can fill in, by their name in the request. */
export type DocumentField = 'document_type' | 'contact_id' | 'reference' | 'date' | 'due_date' | 'currency' | 'prices_are_incl_tax'

const lineFields: readonly string[] = ['description', 'amount', 'price', 'tax_rate_id', 'ledger_account_id'] satisfies LineField[]

/** What Scan and Recognise filled in on a document: each field it filled in and how sure it was. */
export type Remarks = {
  field: (name: DocumentField) => Confidence | undefined
  /** By the id of each line. */
  lines: LineRemarks
}

/**
 * The remarks of a document's recognition. The lines are named by their place
 * among the document's details in row order, as the server read them; they are
 * kept by the line's id so they stay with it when lines move or go.
 */
export function remarksOf(recognition: Recognition | undefined, details: { id?: string; row_order?: number | null }[] = []): Remarks {
  const fields = new Map<string, Confidence>()
  const lines: LineRemarks = {}
  const ordered = [...details].sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
  if (recognition?.state === 'done') {
    for (const { field, confidence } of recognition.fields) {
      const line = /^details\.(\d+)\.([a-z_]+)$/.exec(field)
      if (!line) {
        fields.set(field, confidence)
        continue
      }
      const id = ordered[Number(line[1])]?.id
      const name = line[2] ?? ''
      if (id && lineFields.includes(name)) {
        lines[id] = { ...lines[id], [name]: confidence }
      }
    }
  }
  return { field: (name) => fields.get(name), lines }
}
