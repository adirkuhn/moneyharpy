/**
 * What the document style previews show: the administration's own data
 * wherever it has some. The latest sent invoice gives the customer, lines,
 * reference and totals, so the previews look like the documents customers
 * really get; without one, an example customer and lines stand in, priced
 * in the administration's currency at its standard sales VAT rate.
 */
import { useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { api, ApiError } from '../api/client'
import type { components } from '../api/schema'
import { useAdministration } from '../administrations/queries'
import { useTaxRates, type TaxRate } from '../bookkeeping/queries'
import { addDays, addressLines, today } from '../invoices/paper'
import { useProducts, type Product } from '../products/queries'
import { useInvoiceSettings, useWorkflows, type Workflow } from './queries'
import { customFieldName } from './styleLayout'
import { conditionTagValues } from '../invoices/conditions'
import { draftPaymentReference } from '../invoices/paymentReference'

type SalesInvoice = components['schemas']['SalesInvoice']

export type PreviewLine = { quantity: number; description: string; price: number; percentage: number }
export type PreviewTotals = { subtotal: number; vat: Array<{ percentage: number; amount: number }>; total: number }

export type PreviewData = {
  /** Whether the customer, lines and totals come from the latest invoice or are an example. */
  source: 'invoice' | 'example'
  currency: string
  customer: {
    name: string
    /** The person to address, such as in the email's greeting. */
    contactName: string
    address: string[]
    /** What layout fields print for the customer, such as customer_tax_number or custom_field_5. */
    values: Record<string, string>
  }
  /** What layout fields print for the document itself, such as its custom fields. */
  documentValues: Record<string, string>
  lines: PreviewLine[]
  totals: PreviewTotals
  reference: string
  paymentReference: string
  invoiceNumber: string
  estimateNumber: string
  invoiceDate: string
  dueDate: string
  estimateDueDate: string
  /** The invoice's payment conditions, printed above the payment request. */
  paymentConditions: string
  /** The workflow's email text, its placeholders not yet filled in; empty means the default text. */
  emailSubject: string
  emailBody: string
  /** Products the checkout example offers. */
  products: Array<{ title: string; description: string; price: number; frequency: string }>
}

const sentStates = 'open|pending_payment|reminded|late|paid|uncollectible'

/** The administration's most recent sent invoice, if it has one. */
export function useLatestInvoice(administrationId: string) {
  return useQuery({
    queryKey: ['sales-invoices', administrationId, 'latest-sent'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v2/{administration_id}/sales_invoices', {
        params: { path: { administration_id: administrationId }, query: { filter: `state:${sentStates}`, page: '1', per_page: '1' } },
      })
      if (!response.ok) {
        throw new ApiError(response.status, error)
      }
      return data?.[0] ?? null
    },
  })
}

/** The default active workflow of a kind, whose terms and texts documents of that kind use. */
export function defaultWorkflow(workflows: Workflow[] | undefined, type: 'InvoiceWorkflow' | 'EstimateWorkflow') {
  const ofType = (workflows ?? []).filter((w) => w.type === type && w.active !== false)
  return ofType.find((w) => w.default) ?? ofType[0]
}

/** The standard sales VAT rate: the highest active one for sales invoices. */
export function standardRate(rates: TaxRate[] | undefined) {
  const sales = (rates ?? []).filter((r) => r.active !== false && r.tax_rate_type === 'sales_invoice')
  return sales.reduce<TaxRate | undefined>((best, r) => (!best || Number(r.percentage ?? 0) > Number(best.percentage ?? 0) ? r : best), undefined)
}

export function computeTotals(lines: PreviewLine[]): PreviewTotals {
  const round = (n: number) => Math.round(n * 100) / 100
  const subtotal = round(lines.reduce((sum, l) => sum + l.quantity * l.price, 0))
  const byRate = new Map<number, number>()
  for (const l of lines) {
    byRate.set(l.percentage, (byRate.get(l.percentage) ?? 0) + l.quantity * l.price)
  }
  const vat = [...byRate.entries()].filter(([p]) => p !== 0).map(([percentage, base]) => ({ percentage, amount: round((base * percentage) / 100) }))
  return { subtotal, vat, total: round(subtotal + vat.reduce((s, v) => s + v.amount, 0)) }
}

type Inputs = {
  dt: TFunction
  currency: string
  invoice: SalesInvoice | null | undefined
  workflows: Workflow[] | undefined
  rates: TaxRate[] | undefined
  products: Product[] | undefined
  invoiceNumber: string
  estimateNumber: string
}

/** Puts the previews' data together from what the administration has. */
export function buildPreviewData({ dt, currency, invoice, workflows, rates, products, invoiceNumber, estimateNumber }: Inputs): PreviewData {
  const flow = defaultWorkflow(workflows, 'InvoiceWorkflow')
  const quoteFlow = defaultWorkflow(workflows, 'EstimateWorkflow')
  // The first step (step_order 0, as on Moneybird) holds the payment term or validity.
  const days = (w: Workflow | undefined, fallback: number) =>
    [...(w?.steps ?? [])].sort((a, b) => (a.step_order ?? 0) - (b.step_order ?? 0))[0]?.due_interval ?? fallback
  const date = today()
  const percentageOf = (id: string | null | undefined) => Number(rates?.find((r) => r.id === id)?.percentage ?? 0)
  const shop = (products ?? []).slice(0, 3).map((p) => ({ title: p.title ?? p.description ?? '', description: p.description ?? '', price: Number(p.price ?? 0), frequency: p.frequency_type ?? '' }))
  const common = {
    emailSubject: flow?.invoice_email_subject?.trim() ?? '',
    emailBody: flow?.invoice_email_body?.trim() ?? '',
    estimateNumber,
    estimateDueDate: addDays(date, days(quoteFlow, 30)),
    products: shop,
  }

  if (invoice) {
    const c = invoice.contact
    const person = [c?.firstname, c?.lastname].filter(Boolean).join(' ')
    const name = c?.company_name || person
    const values: Record<string, string> = {
      customer_tax_number: c?.tax_number ?? '', customer_chamber_of_commerce: c?.chamber_of_commerce ?? '', customer_id: c?.customer_id ?? '',
      customer_email: c?.send_invoices_to_email || c?.email || '', customer_phone: c?.phone ?? '',
    }
    for (const f of c?.custom_fields ?? []) {
      values[customFieldName(String(f.id))] = String(f.value ?? '')
    }
    const documentValues: Record<string, string> = {}
    for (const f of invoice.custom_fields ?? []) {
      documentValues[customFieldName(String(f.id))] = String(f.value ?? '')
    }
    const lines = (invoice.details ?? [])
      .filter((d) => !d.is_optional || d.is_selected)
      .map((d) => ({ quantity: Number(d.amount_decimal ?? (Number.parseFloat(String(d.amount ?? '1')) || 1)), description: d.description ?? '', price: Number(d.price ?? 0), percentage: percentageOf(d.tax_rate_id) }))
    const totals: PreviewTotals = {
      subtotal: Number(invoice.total_price_excl_tax ?? 0),
      vat: (invoice.tax_totals ?? []).map((t) => ({ percentage: percentageOf(t.tax_rate_id), amount: Number(t.tax_amount ?? 0) })).filter((v) => v.amount !== 0),
      total: Number(invoice.total_price_incl_tax ?? 0),
    }
    return {
      ...common, source: 'invoice', currency: invoice.currency ?? currency,
      customer: { name, contactName: person || name, address: [c?.attention ?? '', ...addressLines(c ?? {})].filter((l) => l && l !== name), values },
      documentValues, lines, totals, reference: invoice.reference ?? '', paymentReference: invoice.payment_reference || draftPaymentReference(invoice.draft_id),
      invoiceNumber: invoice.invoice_id ?? invoiceNumber, invoiceDate: invoice.invoice_date ?? date,
      dueDate: invoice.due_date ?? addDays(invoice.invoice_date ?? date, days(flow, 14)), paymentConditions: invoice.payment_conditions ?? flow?.payment_conditions ?? '',
    }
  }

  const percentage = Number(standardRate(rates)?.percentage ?? 21)
  const lines = [
    { quantity: 2, description: dt('documentStyle.sample.consultancy'), price: 475.5, percentage },
    { quantity: 1, description: dt('documentStyle.sample.design'), price: 1250, percentage },
  ]
  return {
    ...common, source: 'example', currency,
    customer: {
      name: dt('documentStyle.sample.customer'), contactName: dt('documentStyle.sample.contactName'),
      address: [dt('documentStyle.sample.contactName'), dt('documentStyle.sample.street'), dt('documentStyle.sample.city')],
      values: { customer_tax_number: 'NL000099998B57', customer_chamber_of_commerce: '12345678', customer_id: '1', customer_email: 'ann@example.com', customer_phone: '+31 20 123 4567' },
    },
    documentValues: {}, lines, totals: computeTotals(lines), reference: dt('documentStyle.sample.reference'), paymentReference: draftPaymentReference(null),
    invoiceNumber, invoiceDate: date, dueDate: addDays(date, days(flow, 14)), paymentConditions: flow?.payment_conditions ?? '',
  }
}

/** The previews' data for an administration, in the preview's language; undefined while it loads. */
export function usePreviewData(administrationId: string, dt: TFunction): PreviewData | undefined {
  const administration = useAdministration(administrationId)
  const settings = useInvoiceSettings(administrationId)
  const workflows = useWorkflows(administrationId)
  const rates = useTaxRates(administrationId)
  const latest = useLatestInvoice(administrationId)
  const products = useProducts(administrationId, { query: '', all: false, page: 1 })
  const settled = (q: { isPending: boolean }) => !q.isPending
  if (![administration, settings, workflows, rates, latest].every(settled)) {
    return undefined
  }
  return buildPreviewData({
    dt, currency: administration.data?.currency ?? 'EUR', invoice: latest.data, workflows: workflows.data, rates: rates.data,
    products: products.data?.products, invoiceNumber: settings.data?.invoice_id_example ?? '', estimateNumber: settings.data?.estimate_id_example ?? '',
  })
}

/** What the tags of the payment conditions stand for in the previews, as on the invoice. */
export function previewTagValues(data: PreviewData, language: string, sender: Parameters<typeof conditionTagValues>[0]['sender']) {
  const cents = Math.round(data.totals.total * 100)
  return conditionTagValues({
    language, currency: data.currency, total: cents, unpaid: cents, invoiceDate: data.invoiceDate, dueDate: data.dueDate, invoiceId: data.invoiceNumber,
    reference: data.reference, paymentReference: data.paymentReference, contact: { company_name: data.customer.name }, sender,
  })
}
