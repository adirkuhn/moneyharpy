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

/** Groups an IBAN in blocks of four, as it is printed on paper. */
export function formatIBAN(iban: string | null): string {
  return iban ? iban.replace(/(.{4})(?=.)/g, '$1 ') : ''
}
