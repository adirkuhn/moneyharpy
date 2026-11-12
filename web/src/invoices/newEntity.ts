/**
 * Moneybird's new_entity_field: "Add Category..." or "Add VAT rate..." in a
 * form names the field that asked, such as
 * sales_invoice[details_attributes][1][ledger_account_id]; when the new record
 * is saved, the form comes back with it chosen in that field.
 */

/** The fields of a detail row that can ask for a new record. */
export type NewEntityKey = 'ledger_account_id' | 'tax_rate_id'

/** A record added for a form field, as the page that added it passes back. */
export type NewEntity = { field: string; id: string }

/** The name of a detail row's field, as Moneybird's invoice form writes it. */
export function detailField(index: number, key: NewEntityKey) {
  return `sales_invoice[details_attributes][${index}][${key}]`
}

/** The detail row and field a new_entity_field names, if it names one. */
export function detailOf(field: string): { index: number; key: NewEntityKey } | null {
  const match = /^sales_invoice\[details_attributes\]\[(\d+)\]\[(ledger_account_id|tax_rate_id)\]$/.exec(field)
  return match ? { index: Number(match[1]), key: match[2] as NewEntityKey } : null
}
