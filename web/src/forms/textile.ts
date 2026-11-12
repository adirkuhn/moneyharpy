/**
 * Moneybird's textile texts, such as an invoice's payment conditions:
 * _italic_, *bold* and +underline+, with tags such as {document.due_date}
 * that are filled in where the text is shown. The server fills them in the
 * same way for the PDF and emails.
 */

/** The tags the editor's {tag} menu offers, by group, as Moneybird's. */
export const tagGroups = {
  document: [
    'public_view_code', 'total_price', 'due_date', 'invoice_date', 'invoice_id', 'show_invoice_online_url', 'payment_reference', 'reference',
    'total_price_raw', 'total_unpaid', 'total_unpaid_raw',
  ],
  recipient: [
    'generic_account_name', 'generic_account_number', 'address1', 'address2', 'sepa_iban_account_name', 'attention', 'chamber_of_commerce', 'city',
    'customer_id', 'company_name', 'country', 'credit_card_number', 'email', 'contact_name', 'firstname', 'lastname', 'show_open_invoices_online_url',
    'name', 'phone', 'sepa_bic', 'sepa_iban', 'sepa_mandate_id', 'tax_number', 'zipcode',
  ],
  contact_person: ['department', 'email', 'contact_name', 'firstname', 'lastname', 'phone'],
  company: [
    'bank_account_name', 'bank_account_number', 'address1', 'address2', 'bank_account_bic', 'chamber_of_commerce', 'city', 'company_name', 'country',
    'email', 'phone', 'tax_number', 'zipcode',
  ],
} as const

export type TagGroup = keyof typeof tagGroups
export type TagValues = Record<string, string | null | undefined>

/** Fills in the tags the values know; others stay as they are written. */
export function renderTags(text: string, values: TagValues) {
  return text.replace(/\{([a-z_]+\.[a-z0-9_]+)\}/g, (tag, name: string) => {
    const value = values[name]
    return value === undefined || value === null ? tag : value
  })
}

export type Span = { text: string; bold?: boolean; italic?: boolean; underline?: boolean }

const markers: Record<string, 'bold' | 'italic' | 'underline'> = { '*': 'bold', _: 'italic', '+': 'underline' }

/**
 * Splits a textile text into styled spans. A marker counts when it wraps
 * text that does not start or end with a space; markers that close nothing
 * stay as written. Spans may nest, as *_bold italic_*.
 */
export function parseTextile(text: string, style: Omit<Span, 'text'> = {}): Span[] {
  const spans: Span[] = []
  let plain = ''
  let i = 0
  while (i < text.length) {
    const c = text[i]!
    const kind = markers[c]
    const opens = kind && !style[kind] && i + 1 < text.length && !/\s/.test(text[i + 1]!) && (i === 0 || !/[0-9A-Za-z]/.test(text[i - 1]!))
    if (opens) {
      const end = closing(text, i + 1, c)
      if (end > i + 1) {
        if (plain) {
          spans.push({ text: plain, ...style })
          plain = ''
        }
        spans.push(...parseTextile(text.slice(i + 1, end), { ...style, [kind]: true }))
        i = end + 1
        continue
      }
    }
    plain += c
    i++
  }
  if (plain) {
    spans.push({ text: plain, ...style })
  }
  return merge(spans)
}

/** Where the marker that opened at from closes on the same line, or -1. */
function closing(text: string, from: number, marker: string) {
  for (let j = from; j < text.length; j++) {
    if (text[j] === '\n') {
      return -1
    }
    if (text[j] === marker && !/\s/.test(text[j - 1]!) && (j + 1 === text.length || !/[0-9A-Za-z]/.test(text[j + 1]!))) {
      return j
    }
  }
  return -1
}

function merge(spans: Span[]) {
  const out: Span[] = []
  for (const s of spans) {
    const last = out.at(-1)
    if (last && !!last.bold === !!s.bold && !!last.italic === !!s.italic && !!last.underline === !!s.underline) {
      last.text += s.text
    } else {
      out.push({ ...s })
    }
  }
  return out
}
