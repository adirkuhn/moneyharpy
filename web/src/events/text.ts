import type { TFunction } from 'i18next'

/** SalesInvoice → sales_invoice: how an action names its record type. */
export const snake = (type: string) => type.replace(/([a-z0-9])([A-Z])/g, '$1_$2').toLowerCase()

/** sales_invoice → Sales invoice, for names no translation covers. */
const words = (s: string) => {
  const w = s.replace(/_/g, ' ').trim()
  return w.charAt(0).toUpperCase() + w.slice(1)
}

/** A record type in words, such as Invoice for SalesInvoice. */
export function typeName(type: string, t: TFunction): string {
  return t(`events.types.${type}`, { defaultValue: words(snake(type)) })
}

/**
 * What an event says, after Moneybird's list of events: the record type and
 * what happened to it, such as "Invoice created" or "Payment registered".
 */
export function eventText(entityType: string, action: string, t: TFunction): string {
  // Some actions have their own words at Moneybird, such as "Financial statement added".
  const own = t(`events.actions.${action}`, { defaultValue: '' })
  if (own) {
    return t('events.text', { type: typeName(entityType, t), what: own })
  }
  const prefixes = [snake(entityType), `company_assets_${snake(entityType)}`]
  const prefix = prefixes.find((p) => action.startsWith(`${p}_`))
  const verb = prefix ? action.slice(prefix.length + 1) : action
  const exact = t(`events.verbs.${verb}`, { defaultValue: '' })
  const changed = /^state_changed_to_(.+)$/.exec(verb)
  const what = exact || !changed?.[1]
    ? exact || verb.replace(/_/g, ' ')
    : t('events.verbs.state_changed_to', { state: t(`events.states.${changed[1]}`, { defaultValue: changed[1].replace(/_/g, ' ') }) })
  return prefix ? t('events.text', { type: typeName(entityType, t), what }) : words(what)
}
