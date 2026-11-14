export function countryName(language: string, code: string | null): string {
  if (!code) {
    return ''
  }
  try {
    return new Intl.DisplayNames([language], { type: 'region', fallback: 'code' }).of(code) ?? code
  } catch {
    return code
  }
}

/**
 * The flag of a country as an emoji, made of the two regional indicator
 * symbols of its ISO code, such as 🇳🇱 for NL; empty for anything else.
 * Purely decorative: show it hidden from screen readers.
 */
export function countryFlag(code: string | null): string {
  if (!code || !/^[A-Za-z]{2}$/.test(code)) {
    return ''
  }
  const base = 0x1f1e6 - 'A'.charCodeAt(0)
  return String.fromCodePoint(...[...code.toUpperCase()].map((c) => base + c.charCodeAt(0)))
}

/** Groups an IBAN in blocks of four, as it is printed on paper. */
export function formatIBAN(iban: string | null): string {
  return iban ? iban.replace(/(.{4})(?=.)/g, '$1 ') : ''
}
