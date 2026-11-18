import { type FormEvent, useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { periodOfParam } from '../vat/period'
import { formatAmount, type LinkScope, type LinkSuggestion, type Mutation, useLink, useLinkSuggestions } from './queries'

const scopes: LinkScope[] = ['all', 'sales_invoices', 'documents', 'ledger_accounts']

/** The colour of a document's state dot, as Moneybird's labels: late red, open amber, paid green. */
function stateTone(state: string) {
  if (['late', 'reminded', 'expired'].includes(state)) return 'is-late'
  if (state === 'paid') return 'is-paid'
  return 'is-open'
}

type Props = { administrationId: string; mutation: Mutation; onDone: () => void }

/**
 * Moneybird's linking of a transaction, in its card: a search over the
 * invoices, incoming documents and categories with the best matches first
 * and what linking each would book, new purchase invoices and receipts,
 * the amount to link and, for a category, a description.
 */
export function LinkPanel({ administrationId, mutation: m, onDone }: Props) {
  const { t, i18n } = useTranslation()
  const id = m.id ?? ''
  const [typed, setTyped] = useState('')
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState<LinkScope>('all')
  const suggestions = useLinkSuggestions(administrationId, id, query, scope)
  const link = useLink(administrationId, id)
  const [chosen, setChosen] = useState<LinkSuggestion | null>(null)
  // Null until typed: the chosen match's price stands in.
  const [amount, setAmount] = useState<string | null>(null)
  const [description, setDescription] = useState('')
  const listId = useId()
  const cur = m.currency ?? 'EUR'
  const plain = (v: string | number | null | undefined) => formatAmount(v, cur, i18n.language)
  const signed = (v: string | number | null | undefined) => {
    const n = Number(v ?? 0)
    return formatAmount(Math.abs(n), cur, i18n.language).replace(/^(\D*)/, `$1${n < 0 ? '-' : '+'}`)
  }

  // A search waits for the typing to pause.
  useEffect(() => {
    const timer = setTimeout(() => setQuery(typed.trim()), 250)
    return () => clearTimeout(timer)
  }, [typed])

  const list = suggestions.data ?? []
  const selected = chosen ?? list[0] ?? null
  const shownAmount = amount ?? (selected ? String(Math.abs(Number(selected.price))) : '')
  const open = Math.abs(Number(m.amount_open ?? 0))
  const left = Math.max(0, open - Math.abs(Number(shownAmount.replace(',', '.')) || 0))
  const choose = (s: LinkSuggestion) => {
    setChosen(s)
    setAmount(null)
  }
  const errors = formErrors(link.error, { price_base: t('bank.link.amount'), description: t('bank.link.description') })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!selected) {
      return
    }
    const value = Number(shownAmount.replace(',', '.'))
    const sign = Number(selected.price) < 0 ? -1 : 1
    link.mutate(
      {
        booking_type: selected.booking_type, booking_id: selected.booking_id, price_base: Number.isFinite(value) ? String(value * sign) : shownAmount,
        ...(selected.booking_type === 'LedgerAccount' && description.trim() ? { description: description.trim() } : {}),
      },
      { onSuccess: onDone },
    )
  }
  const makeNew = (kind: 'NewPurchaseInvoice' | 'NewReceipt') => link.mutate({ booking_type: kind }, { onSuccess: onDone })

  return (
    <form className="link-panel" onSubmit={submit} noValidate aria-label={t('bank.link.title')}>
      <div className="link-panel__search">
        <select aria-label={t('bank.link.scope')} value={scope} onChange={(e) => setScope(e.target.value as LinkScope)}>
          {scopes.map((s) => <option key={s} value={s}>{t(`bank.link.scopes.${s}`)}</option>)}
        </select>
        <input
          type="search"
          aria-label={t('bank.link.find')}
          placeholder={t('bank.link.find')}
          value={typed}
          aria-controls={listId}
          onChange={(e) => setTyped(e.target.value)}
        />
        <button type="button" className="btn btn--secondary btn--small btn--with-icon" disabled={link.isPending} onClick={() => makeNew('NewPurchaseInvoice')}>
          <ListIcon kind="plus" />
          <span>{t('bank.link.newPurchaseInvoice')}</span>
        </button>
        <button type="button" className="btn btn--secondary btn--small btn--with-icon" disabled={link.isPending} onClick={() => makeNew('NewReceipt')}>
          <ListIcon kind="plus" />
          <span>{t('bank.link.newReceipt')}</span>
        </button>
      </div>
      <ul id={listId} className="link-panel__results" role="radiogroup" aria-label={t('bank.link.results')}>
        {list.length === 0 && !suggestions.isFetching && <li className="link-panel__empty">{t('bank.link.nothing')}</li>}
        {list.map((s) => {
          const isSelected = selected?.booking_type === s.booking_type && selected.booking_id === s.booking_id
          const reference = s.booking_type === 'VatDocument' ? periodOfParam(s.reference, t, i18n.language) : s.reference
          const name = s.label ? `${s.label} ${[reference, s.contact_name].filter(Boolean).join(' · ')}` : s.reference
          return (
            <li key={`${s.booking_type}-${s.booking_id}`}>
              <label className={`link-panel__result${isSelected ? ' is-selected' : ''}`}>
                <input type="radio" className="sr-only" name={`${listId}-choice`} checked={isSelected} onChange={() => choose(s)} />
                <span className={`link-panel__dot ${s.label ? stateTone(s.state) : 'is-category'}`} aria-hidden="true" />
                <span className="link-panel__summary">
                  <span className="link-panel__name">
                    {s.label ? <><strong>{s.label}</strong> {[reference, s.contact_name].filter(Boolean).join(' · ')}</> : name}
                  </span>
                  {s.label && <span className="link-panel__line">{[s.date, plain(s.total)].filter(Boolean).join(' · ')}</span>}
                  {s.payment_reference && <span className="link-panel__line">{t('bank.feed.paymentReference', { reference: s.payment_reference })}</span>}
                  {s.details && <span className="link-panel__line link-panel__details">{s.details}</span>}
                </span>
                <span className="link-panel__price">{signed(s.price)}</span>
              </label>
            </li>
          )
        })}
      </ul>
      {selected?.booking_type === 'LedgerAccount' && (
        <textarea className="link-panel__description" aria-label={t('bank.link.description')} placeholder={t('bank.link.descriptionPlaceholder')} maxLength={1000} value={description} onChange={(e) => setDescription(e.target.value)} />
      )}
      <Problems messages={[...errors.general, ...(errors.field('price_base') ?? []), ...(errors.field('description') ?? [])]} />
      <div className="link-panel__footer">
        <div className="link-panel__buttons">
          <button className="btn btn--primary" disabled={!selected || link.isPending}>{t('bank.link.submit')}</button>
          <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
        </div>
        <label className="link-panel__amount">
          <span className="sr-only">{t('bank.link.amount')}</span>
          <span aria-hidden="true">{new Intl.NumberFormat(i18n.language, { style: 'currency', currency: cur }).formatToParts(0).find((part) => part.type === 'currency')?.value}</span>
          <input inputMode="decimal" maxLength={16} value={shownAmount} onChange={(e) => setAmount(e.target.value)} />
        </label>
        <span className="link-panel__left">{t('bank.link.left', { amount: plain(left) })}</span>
      </div>
    </form>
  )
}
