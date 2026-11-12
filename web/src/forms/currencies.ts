import type { TFunction } from 'i18next'

/** The currencies Moneybird offers first, above a separator. */
export const popularCurrencies = ['EUR', 'USD', 'GBP']

/** The other currencies Moneybird offers for an invoice, by code. */
export const otherCurrencies = [
  'AED', 'AFN', 'ALL', 'AMD', 'ANG', 'AOA', 'ARS', 'AUD', 'AWG', 'AZN', 'BAM', 'BBD', 'BDT', 'BGN', 'BHD', 'BIF',
  'BMD', 'BND', 'BOB', 'BRL', 'BSD', 'BTC', 'BTN', 'BWP', 'BYN', 'BYR', 'BZD', 'CAD', 'CDF', 'CHF', 'CLF', 'CLP',
  'CNY', 'COP', 'CRC', 'CUC', 'CUP', 'CVE', 'CZK', 'DJF', 'DKK', 'DOP', 'DZD', 'EEK', 'EGP', 'ERN', 'ETB', 'FJD',
  'FKP', 'GEL', 'GHS', 'GIP', 'GMD', 'GNF', 'GTQ', 'GYD', 'HKD', 'HNL', 'HRK', 'HTG', 'HUF', 'IDR', 'ILS', 'INR',
  'IQD', 'IRR', 'ISK', 'JEP', 'JMD', 'JOD', 'JPY', 'KES', 'KGS', 'KHR', 'KMF', 'KPW', 'KRW', 'KWD', 'KYD', 'KZT',
  'LAK', 'LBP', 'LKR', 'LRD', 'LSL', 'LTL', 'LVL', 'LYD', 'MAD', 'MDL', 'MGA', 'MKD', 'MMK', 'MNT', 'MOP', 'MRO',
  'MTL', 'MUR', 'MVR', 'MWK', 'MXN', 'MYR', 'MZN', 'NAD', 'NGN', 'NIO', 'NOK', 'NPR', 'NZD', 'OMR', 'PAB', 'PEN',
  'PGK', 'PHP', 'PKR', 'PLN', 'PYG', 'QAR', 'RON', 'RSD', 'RUB', 'RWF', 'SAR', 'SBD', 'SCR', 'SDG', 'SEK', 'SGD',
  'SHP', 'SLL', 'SOS', 'SRD', 'STD', 'SVC', 'SYP', 'SZL', 'THB', 'TJS', 'TMT', 'TND', 'TOP', 'TRY', 'TTD', 'TWD',
  'TZS', 'UAH', 'UGX', 'UYU', 'UZS', 'VEF', 'VND', 'VUV', 'WST', 'XAF', 'XAG', 'XAU', 'XCD', 'XCG', 'XDR', 'XOF',
  'XPF', 'YER', 'ZAR', 'ZMK', 'ZMW', 'ZWL',
]

export type CurrencyOption = { value: string; label: string; disabled?: boolean }

/** "USD - United States Dollar": a currency as Moneybird's select shows it. */
export const currencyLabel = (code: string, t: TFunction) => `${code} - ${t(`currencies.${code}`, { defaultValue: code })}`

/**
 * Moneybird's currency select: EUR, USD and GBP, a disabled separator, then
 * every other currency by code. A currency kept from elsewhere that is not in
 * the list is offered too, so it still shows.
 */
export function currencyOptions(t: TFunction, current?: string): CurrencyOption[] {
  const known = [...popularCurrencies, ...otherCurrencies]
  return [
    ...popularCurrencies.map((code) => ({ value: code, label: currencyLabel(code, t) })),
    { value: '', label: '----------', disabled: true },
    ...otherCurrencies.map((code) => ({ value: code, label: currencyLabel(code, t) })),
    ...(current && !known.includes(current) ? [{ value: current, label: currencyLabel(current, t) }] : []),
  ]
}
