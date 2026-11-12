import { describe, expect, it } from 'vitest'
import { en } from '../locales/en'
import { nl } from '../locales/nl'
import { otherCurrencies, popularCurrencies } from './currencies'

describe('the currencies', () => {
  it("lists Moneybird's currencies, sorted, without the popular ones twice", () => {
    expect(otherCurrencies).toHaveLength(166)
    expect([...otherCurrencies].sort()).toEqual(otherCurrencies)
    expect(otherCurrencies.filter((c) => popularCurrencies.includes(c))).toEqual([])
    expect(otherCurrencies).toEqual(expect.arrayContaining(['BTC', 'XAU', 'ZWL']))
  })

  it('names every currency in English and Dutch', () => {
    for (const code of [...popularCurrencies, ...otherCurrencies]) {
      expect(en.currencies[code as keyof typeof en.currencies], code).toBeTruthy()
      expect(nl.currencies[code as keyof typeof nl.currencies], code).toBeTruthy()
    }
    expect(nl.currencies.USD).toBe('Amerikaanse dollar')
  })
})
