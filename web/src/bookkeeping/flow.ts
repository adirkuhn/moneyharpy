import { useSearchParams } from 'react-router-dom'
import { returnPath } from '../settings/CustomFieldFormPage'

/** Where adding a category was started, and where it goes back to. */
export type CategoryFlow = {
  /** The page to go back to: the one that asked, or the categories of the bookkeeping settings. */
  returnTo: string
  /** Started from an invoice, as with Moneybird's "Add Category..." in the invoice details. */
  forInvoice: boolean
  /** The query that carries the flow to the next page, with its "?", or empty. */
  query: string
}

/**
 * Reads the flow of Moneybird's Add category pages from the address: return_to
 * names the page to go back to (only one of this administration) and source the
 * kind of document the category is for.
 */
export function useCategoryFlow(administrationId: string): CategoryFlow {
  const [params] = useSearchParams()
  const fallback = `/${administrationId}/settings/bookkeeping#categories`
  const requested = params.get('return_to')
  const returnTo = requested && returnPath(administrationId, requested) === requested ? requested : ''
  const forInvoice = params.get('source') === 'sales_invoice'
  const carried = new URLSearchParams()
  if (returnTo) {
    carried.set('return_to', returnTo)
  }
  if (forInvoice) {
    carried.set('source', 'sales_invoice')
  }
  const query = carried.toString()
  return { returnTo: returnTo || fallback, forInvoice, query: query ? `?${query}` : '' }
}
