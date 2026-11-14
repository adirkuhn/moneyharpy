import type { components } from '../api/schema'
import type { Confidence, RemarkSource } from '../forms/Remark'
import type { LineField, LineRemarks } from '../invoices/DetailsEditor'

export type Recognition = components['schemas']['Recognition']
type RecognizedField = components['schemas']['RecognizedField']

/** The fields of a document Scan and Recognise can fill in, by their name in the request. */
export type DocumentField = 'document_type' | 'contact_id' | 'reference' | 'date' | 'due_date' | 'currency' | 'prices_are_incl_tax'

const lineFields: readonly string[] = ['description', 'amount', 'price', 'tax_rate_id', 'ledger_account_id'] satisfies LineField[]

/** What Scan and Recognise filled in on a document: each field it filled in and how sure it was. */
export type Remarks = {
  field: (name: DocumentField) => Confidence | undefined
  /** The earlier document a field's remembered value came from. */
  source: (name: DocumentField) => RemarkSource | undefined
  /** By the id of each line. */
  lines: LineRemarks
  /** The earlier document each line's remembered values came from, by the id of the line. */
  lineSources: Record<string, RemarkSource>
}

type Options = {
  /** Where the documents remembered values came from are. */
  administrationId?: string
  /** Only the values remembered of the contact's earlier documents. */
  rememberedOnly?: boolean
}

/** A line's source is that of its category, else of its VAT rate, else of any of its fields. */
const sourceRank = (name: string) => {
  const rank = ['ledger_account_id', 'tax_rate_id'].indexOf(name)
  return rank < 0 ? 2 : rank
}

/**
 * The remarks of a document's recognition. The lines are named by their place
 * among the document's details in row order, as the server read them; they are
 * kept by the line's id so they stay with it when lines move or go.
 */
export function remarksOf(recognition: Recognition | undefined, details: { id?: string; row_order?: number | null }[] = [], options: Options = {}): Remarks {
  const fields = new Map<string, Confidence>()
  const sources = new Map<string, RemarkSource>()
  const lines: LineRemarks = {}
  const lineSources: Record<string, RemarkSource> = {}
  const lineSourceRank: Record<string, number> = {}
  const ordered = [...details].sort((a, b) => (a.row_order ?? 0) - (b.row_order ?? 0))
  const sourceOf = (f: RecognizedField): RemarkSource | undefined =>
    f.source_document_id && options.administrationId !== undefined
      ? { to: `/${options.administrationId}/documents/${f.source_document_id}`, reference: f.source_document_reference ?? null }
      : undefined
  if (recognition?.state === 'done') {
    for (const f of recognition.fields) {
      if (options.rememberedOnly && f.reason !== 'previous_document') {
        continue
      }
      const { field, confidence } = f
      const source = sourceOf(f)
      const line = /^details\.(\d+)\.([a-z_]+)$/.exec(field)
      if (!line) {
        fields.set(field, confidence)
        if (source) {
          sources.set(field, source)
        }
        continue
      }
      const id = ordered[Number(line[1])]?.id
      const name = line[2] ?? ''
      if (id && lineFields.includes(name)) {
        lines[id] = { ...lines[id], [name]: confidence }
        if (source && (lineSourceRank[id] === undefined || sourceRank(name) < (lineSourceRank[id] ?? 0))) {
          lineSources[id] = source
          lineSourceRank[id] = sourceRank(name)
        }
      }
    }
  }
  return { field: (name) => fields.get(name), source: (name) => sources.get(name), lines, lineSources }
}
