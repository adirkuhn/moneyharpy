import type { TagValues } from '../forms/textile'
import { draftPaymentReference, shortPaymentReference } from './paymentReference'

/** Records as the API gives them; fields may be missing, null or, for a few, typed loosely. */
type Fields = { [field: string]: unknown }
type Sender = Fields & { custom_fields?: { id?: string | null; value?: string | null }[] | null }

export type ConditionsInput = {
  language: string
  currency: string
  /** Amounts in cents. */
  total: number
  unpaid: number
  invoiceDate: string
  dueDate: string
  invoiceId?: string | null
  draftId?: number | string | null
  reference?: string | null
  paymentReference?: string | null
  contact?: Fields | null
  contactPerson?: Fields | null
  sender?: Sender | null
}

/** A field as text: empty unless it is a string. */
const text = (v: unknown) => (typeof v === 'string' ? v : '')
const name = (p: Fields | null | undefined) => [text(p?.firstname), text(p?.lastname)].filter(Boolean).join(' ')

/** A date as the document's tags print it, 26-11-2026, as the server does. */
export function tagDate(iso: string) {
  const [y, m, d] = iso.split('-')
  return y && m && d ? `${d}-${m}-${y}` : ''
}

/** An amount's number as the server prints it: 1.234,50 in Dutch, 1,234.50 otherwise. */
export function tagNumber(cents: number, language: string) {
  const abs = Math.abs(Math.round(cents))
  const whole = String(Math.floor(abs / 100)).replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  let text = `${whole}.${String(abs % 100).padStart(2, '0')}`
  if (language.startsWith('nl')) {
    text = text.replace(/[.,]/g, (c) => (c === '.' ? ',' : '.'))
  }
  return cents < 0 ? `-${text}` : text
}

/** An amount with its currency as the server prints it: € 1.234,50 in Dutch, €1,234.50 otherwise. */
export function tagMoney(cents: number, currency: string, language: string) {
  let symbol = ({ EUR: '€', USD: '$', GBP: '£' } as Record<string, string>)[currency] ?? `${currency} `
  if (language.startsWith('nl')) {
    symbol = `${symbol.trim()} `
  }
  const text = tagNumber(Math.abs(cents), language)
  return cents < 0 ? `-${symbol}${text}` : `${symbol}${text}`
}

/**
 * What the tags of an invoice's payment conditions stand for, by the same
 * rules the server fills them in on the PDF and in emails. A draft shows
 * Moneybird's RF..CONCEPT reference until it is sent.
 */
export function conditionTagValues(i: ConditionsInput): TagValues {
  const c = i.contact
  const p = i.contactPerson
  const s = i.sender
  const reference = i.paymentReference ? shortPaymentReference(i.paymentReference) : draftPaymentReference(i.draftId)
  const recipientName = text(c?.company_name) || name(c)
  return {
    'document.public_view_code': '', 'document.total_price': tagMoney(i.total, i.currency, i.language), 'document.due_date': tagDate(i.dueDate),
    'document.invoice_date': tagDate(i.invoiceDate), 'document.invoice_id': i.invoiceId ?? '', 'document.show_invoice_online_url': '',
    'document.payment_reference': reference, 'document.reference': i.reference ?? '', 'document.total_price_raw': tagNumber(i.total, i.language),
    'document.total_unpaid': tagMoney(i.unpaid, i.currency, i.language), 'document.total_unpaid_raw': tagNumber(i.unpaid, i.language),
    'recipient.generic_account_name': '', 'recipient.generic_account_number': text(c?.bank_account), 'recipient.address1': text(c?.address1),
    'recipient.address2': text(c?.address2), 'recipient.sepa_iban_account_name': text(c?.sepa_iban_account_name), 'recipient.attention': text(c?.send_invoices_to_attention),
    'recipient.chamber_of_commerce': text(c?.chamber_of_commerce), 'recipient.city': text(c?.city), 'recipient.customer_id': text(c?.customer_id),
    'recipient.company_name': text(c?.company_name), 'recipient.country': text(c?.country), 'recipient.credit_card_number': '',
    'recipient.email': text(c?.send_invoices_to_email), 'recipient.contact_name': name(c), 'recipient.firstname': text(c?.firstname),
    'recipient.lastname': text(c?.lastname), 'recipient.show_open_invoices_online_url': '', 'recipient.name': recipientName, 'recipient.phone': text(c?.phone),
    'recipient.sepa_bic': text(c?.sepa_bic), 'recipient.sepa_iban': text(c?.sepa_iban), 'recipient.sepa_mandate_id': text(c?.sepa_mandate_id),
    'recipient.tax_number': text(c?.tax_number), 'recipient.zipcode': text(c?.zipcode),
    'contact_person.department': text(p?.department), 'contact_person.email': text(p?.email), 'contact_person.contact_name': name(p),
    'contact_person.firstname': text(p?.firstname), 'contact_person.lastname': text(p?.lastname), 'contact_person.phone': text(p?.phone),
    'company.bank_account_name': text(s?.bank_account_name), 'company.bank_account_number': text(s?.bank_account_number), 'company.address1': text(s?.address1),
    'company.address2': text(s?.address2), 'company.bank_account_bic': text(s?.bank_account_bic), 'company.chamber_of_commerce': text(s?.chamber_of_commerce),
    'company.city': text(s?.city), 'company.company_name': text(s?.company_name), 'company.country': text(s?.country), 'company.email': text(s?.email),
    'company.phone': text(s?.phone), 'company.tax_number': text(s?.tax_number), 'company.zipcode': text(s?.zipcode),
    ...Object.fromEntries((s?.custom_fields ?? []).map((f) => [`company.custom_field_${f.id}`, f.value ?? ''])),
  }
}
