/**
 * Moneybird's payment reference: an ISO 11649 RF creditor reference, such as
 * RF13PV96X5KP, that customers quote so a bank transaction can be matched
 * to its invoice. The API keeps it padded to 25 characters
 * (RF130000000000000PV96X5KP); invoices print the short form. A draft has
 * none yet and shows RF..CONCEPT with its draft number instead.
 */

/** The two check digits of an RF reference with this body. */
export function rfCheckDigits(body: string) {
  const digits = `${body}RF00`.toUpperCase().replace(/[0-9A-Z]/g, (c) => String(parseInt(c, 36)))
  let rest = 0
  for (const d of digits) {
    rest = (rest * 10 + Number(d)) % 97
  }
  return String(98 - rest).padStart(2, '0')
}

/** Upper case without spaces, as references are compared. */
export function normalizeReference(s: string) {
  return s.replace(/\s+/g, '').toUpperCase()
}

/** The printed form of a reference: an RF reference without its zero padding; anything else as it is. */
export function shortPaymentReference(ref: string) {
  const r = normalizeReference(ref)
  const m = /^RF(\d{2})0*([0-9A-Z]+)$/.exec(r)
  return m ? `RF${m[1]}${m[2]}` : ref.trim()
}

/** What a draft shows for its payment reference, as Moneybird does: RF95CONCEPT42 for draft 42. */
export function draftPaymentReference(draftId?: number | string | null) {
  const body = `CONCEPT${draftId ?? ''}`
  return `RF${rfCheckDigits(body)}${body}`
}

/** The forms a reference is quoted in: the short one and, for an RF reference, the padded one. */
function forms(ref: string) {
  const short = normalizeReference(shortPaymentReference(ref))
  const m = /^RF(\d{2})([0-9A-Z]+)$/.exec(short)
  const long = m && m[2]!.length < 21 ? `RF${m[1]}${m[2]!.padStart(21, '0')}` : ''
  return [short, normalizeReference(ref), long].filter((f) => f.length >= 4)
}

/**
 * Whether a transaction's description quotes the reference, however it is
 * spaced or cased, in its short or its padded form.
 */
export function containsPaymentReference(text: string | null | undefined, ref: string | null | undefined) {
  if (!text || !ref) {
    return false
  }
  const message = normalizeReference(text)
  return forms(ref).some((f) => message.includes(f))
}
