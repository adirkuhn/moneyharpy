import { useSearchParams } from 'react-router-dom'
import type { NewEntity } from '../invoices/newEntity'
import { returnPath } from '../settings/CustomFieldFormPage'

/**
 * What a page gone back to opens: an invoice opens its details again, as
 * Moneybird's return_to with ?open=details does, with the record just added
 * chosen in the field that asked for it (Moneybird's new_entity_field).
 */
export type ReturnState = { details: true; newEntity?: NewEntity }

/** Where adding a category was started, and where it goes back to. */
export type CategoryFlow = {
  /** The page to go back to: the one that asked, or the categories of the bookkeeping settings. */
  returnTo: string
  /** What the page gone back to opens without a new category, as when cancelling. */
  returnState?: ReturnState
  /** What the page gone back to opens once categories were added, the first of `added` chosen where it was asked for. */
  stateAfter: (added: Array<{ id?: string | null; parent_id?: string | number | null }>) => ReturnState | undefined
  /** Started from an invoice, as with Moneybird's "Add Category..." in the invoice details. */
  forInvoice: boolean
  /** The query that carries the flow to the next page, with its "?", or empty. */
  query: string
}

/**
 * The page that asked for a new record, from Moneybird's return_to (only a
 * page of this administration), and the field it is for, from
 * new_entity_field.
 */
export function useReturnTo(administrationId: string) {
  const [params] = useSearchParams()
  const requested = params.get('return_to')
  const returnTo = requested && returnPath(administrationId, requested) === requested ? requested : ''
  return { returnTo, newEntityField: (returnTo && params.get('new_entity_field')) || '' }
}

/** What the page that asked gets back: its details open, with the new record on the field that asked. */
export function backState(newEntityField: string, id: string | null | undefined): ReturnState {
  return newEntityField && id ? { details: true, newEntity: { field: newEntityField, id } } : { details: true }
}

/**
 * Reads the flow of Moneybird's Add category pages from the address: return_to
 * names the page to go back to, source the kind of document the category is
 * for and new_entity_field the field that asked for it.
 */
export function useCategoryFlow(administrationId: string): CategoryFlow {
  const [params] = useSearchParams()
  const { returnTo, newEntityField } = useReturnTo(administrationId)
  const fallback = `/${administrationId}/settings/bookkeeping#categories`
  const forInvoice = params.get('source') === 'sales_invoice'
  const carried = new URLSearchParams()
  if (returnTo) {
    carried.set('return_to', returnTo)
  }
  if (forInvoice) {
    carried.set('source', 'sales_invoice')
  }
  if (newEntityField) {
    carried.set('new_entity_field', newEntityField)
  }
  const query = carried.toString()
  const backToInvoice = forInvoice && !!returnTo
  return {
    returnTo: returnTo || fallback,
    ...(backToInvoice ? { returnState: { details: true as const } } : {}),
    // Of several categories added at once, the first that is not the parent of another is chosen.
    stateAfter: (added) => {
      if (!backToInvoice) {
        return undefined
      }
      const parents = new Set(added.map((a) => String(a.parent_id ?? '')))
      return backState(newEntityField, (added.find((a) => !parents.has(String(a.id ?? ''))) ?? added[0])?.id)
    },
    forInvoice,
    query: query ? `?${query}` : '',
  }
}
