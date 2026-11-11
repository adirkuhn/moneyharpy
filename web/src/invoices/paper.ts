/**
 * What the invoice document shows, worked out from the invoice, its document
 * style and the sender's identity, so the editor can draw it as on paper.
 */
import type { Line } from '../invoicing/queries'
import type { Identity } from '../settings/queries'

/** The sender block Moneybird and our server start every document style with. */
export const defaultSenderLines: Line[] = [
  { field: 'company_name', bold: true }, { field: 'address' }, { field: 'newline' }, { field: 'email' }, { field: 'phone' },
  { field: 'newline' }, { field: 'chamber_of_commerce', label: true }, { field: 'tax_number', label: true }, { field: 'bank_account_number', label: true },
]

/** One entry of the sender block: a blank line, or a field's text over one or more lines. */
export type SenderEntry = { field: 'newline' } | { field: string; bold: boolean; label: boolean; text: string[] }

type Sender = Partial<Pick<Identity, 'company_name' | 'address1' | 'address2' | 'zipcode' | 'city' | 'email' | 'phone' | 'chamber_of_commerce' | 'tax_number' | 'bank_account_number' | 'bank_account_bic' | 'bank_account_name'>> & { website?: string | null }

/** The lines of an address: street lines, then the postcode with the city. */
export function addressLines(a: { address1?: string | null; address2?: string | null; zipcode?: string | null; city?: string | null }) {
  return [a.address1, a.address2, [a.zipcode, a.city].filter(Boolean).join(' ')].filter((l): l is string => !!l && l.trim() !== '')
}

function fieldText(field: string, sender: Sender): string[] {
  if (field === 'address') {
    return addressLines(sender)
  }
  const value = (sender as Record<string, string | null | undefined>)[field]
  return value ? [value] : []
}

/**
 * The sender block as a document style lays it out. Fields without a value are
 * left out, as on the printed invoice; blank lines are kept.
 */
export function senderEntries(lines: Array<Record<string, string | boolean>> | undefined, sender: Sender | undefined): SenderEntry[] {
  const layout = lines ?? defaultSenderLines
  return layout.flatMap((l): SenderEntry[] => {
    const field = String(l.field ?? '')
    if (field === 'newline') {
      return [{ field }]
    }
    const text = sender ? fieldText(field, sender) : []
    return text.length ? [{ field, bold: l.bold === true, label: l.label === true, text }] : []
  })
}

/** Today in the browser's time zone, as yyyy-mm-dd. */
export function today() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

const dayMs = 24 * 60 * 60 * 1000
const utc = (iso: string) => Date.UTC(Number(iso.slice(0, 4)), Number(iso.slice(5, 7)) - 1, Number(iso.slice(8, 10)))

/** The date a number of days after an yyyy-mm-dd date. */
export function addDays(iso: string, days: number) {
  return new Date(utc(iso) + days * dayMs).toISOString().slice(0, 10)
}

/** The whole days from one yyyy-mm-dd date to another. */
export function daysBetween(from: string, to: string) {
  return Math.round((utc(to) - utc(from)) / dayMs)
}

/** A date as the document prints it: 25-11-2026 in Dutch, 11/25/2026 in English. */
export function documentDate(iso: string, language: string) {
  return new Date(`${iso}T00:00:00`).toLocaleDateString(language, { day: '2-digit', month: '2-digit', year: 'numeric' })
}

/** A VAT percentage as the document prints it, such as 21%. */
export function percentText(percentage: string | number | null | undefined) {
  return `${Number(percentage ?? 0)}%`
}
