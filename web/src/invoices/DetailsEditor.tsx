import { type KeyboardEvent, useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { bookable, formatAmount } from '../banking/queries'
import type { LedgerAccount, TaxRate } from '../bookkeeping/queries'
import { currencyOptions } from '../forms/currencies'
import type { FormErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { PageIcon } from '../layout/pageIcons'
import { ActionMenu } from '../list/ListActions'
import { type Product, useProductSearch } from '../products/queries'
import { periodPresets, periodRange, periodText, presetPeriod, toPeriod } from './details'
import type { LineRow } from './LinesEditor'
import { EditableDocumentForm } from './EditableDocumentForm'
import type { NewEntityKey } from './newEntity'
import { today } from './paper'
import { type TotalRow, TotalsTable } from './TotalsTable'
import { categoryOption } from '../bookkeeping/queries'

/** A detail row as the editor keeps it: a line with its period and the product it came from. */
export type DetailRow = LineRow & { period: string; product_id: string; /** The period the server keeps, so clearing it is sent. */ savedPeriod?: string }

/** The fields "Apply to details below" copies to the rows under a changed one. */
type Applicable = 'period' | 'tax_rate_id' | 'ledger_account_id'

/** What choosing an option of a select adds instead of choosing a value. */
const addOption = '__add'

type Props = {
  administrationId: string
  rows: DetailRow[]
  rates: TaxRate[]
  accounts: LedgerAccount[]
  currency: string
  /** Whether the prices typed include VAT, Moneybird's "Prices are". */
  pricesInclTax: boolean
  onPricesInclTax: (inclTax: boolean) => void
  onCurrency: (currency: string) => void
  /** A sent invoice keeps its currency, as Moneybird's select is disabled then. */
  currencyLocked?: boolean
  /** Each row's total in cents, in the order of rows. */
  lineTotals: number[]
  /** Whether each row's price and amount can be read. */
  invalid: { price: boolean; amount: boolean }[]
  totals: TotalRow[]
  discount: string
  discountInvalid: boolean
  errors: FormErrors
  onChange: (index: number, change: Partial<DetailRow>) => void
  onRemove: (index: number) => void
  onAdd: (row?: Partial<DetailRow>) => void
  /** Puts the details back as they were when the editor opened, and closes it. */
  onCancel: () => void
  onMove: (from: number, to: number) => void
  onApplyBelow: (index: number, change: Partial<DetailRow>) => void
  onDiscount: (value: string) => void
  /** Saves the invoice and closes the editor, as Moneybird's blue Save. */
  onSave: () => void
  /** While the invoice is being saved. */
  saving?: boolean
  /**
   * "Add Category..." and "Add VAT rate..." of a row: go to Moneybird's Add
   * category or Add VAT rate in this tab, which comes back to the invoice with
   * the new one on that row; the editor around makes sure nothing typed is lost.
   */
  onAddEntity: (index: number, field: NewEntityKey) => void
}

/**
 * Moneybird's invoice details editor: sortable rows of amount, description
 * with product search and period, price, total, VAT rate and category, with
 * discounts and the totals underneath.
 */
export function DetailsEditor(props: Props) {
  const { administrationId, rows, rates, accounts, currency, lineTotals, invalid, totals, discount, discountInvalid, errors } = props
  const { t, i18n } = useTranslation()
  const d = (key: string, options: Record<string, unknown> = {}) => t(`invoices.details.${key}`, options)
  const hintId = useId()
  const [changed, setChanged] = useState<Record<string, Applicable[]>>({})
  const [discountShown, setDiscountShown] = useState(discount.trim() !== '')
  const [dragging, setDragging] = useState<string | null>(null)
  const [focusGrip, setFocusGrip] = useState<string | null>(null)
  const grips = useRef(new Map<string, HTMLButtonElement>())
  const discountInput = useRef<HTMLInputElement>(null)
  const groups = bookable(accounts, 'sales_invoice')
  const money = (cents: number) => formatAmount(cents / 100, currency, i18n.language)

  useEffect(() => {
    if (focusGrip) {
      grips.current.get(focusGrip)?.focus()
      setFocusGrip(null)
    }
  }, [focusGrip, rows])

  const mark = (key: string, field: Applicable, on: boolean) =>
    setChanged((c) => ({ ...c, [key]: on ? [...new Set([...(c[key] ?? []), field])] : (c[key] ?? []).filter((f) => f !== field) }))
  const set = (i: number, row: DetailRow, field: Applicable, value: string) => {
    props.onChange(i, { [field]: value })
    mark(row.key, field, true)
  }
  const applyBelow = (i: number, row: DetailRow, field: Applicable) => {
    props.onApplyBelow(i, { [field]: row[field] })
    mark(row.key, field, false)
  }
  const applyLink = (i: number, row: DetailRow, field: Applicable) =>
    i < rows.length - 1 && changed[row.key]?.includes(field) && (
      <button type="button" className="link-button details-editor__apply" onClick={() => applyBelow(i, row, field)}>{d('applyBelow')}</button>
    )
  const move = (from: number, to: number) => {
    const row = rows[from]
    if (row && to >= 0 && to < rows.length) {
      props.onMove(from, to)
      setFocusGrip(row.key)
    }
  }
  const onGripKey = (i: number) => (e: KeyboardEvent) => {
    if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
      e.preventDefault()
      move(i, e.key === 'ArrowUp' ? i - 1 : i + 1)
    }
  }
  const cell = (i: number, field: string, invalidInput = false) => {
    const problems = errors.field(`details_attributes.${i}.${field}`)
    const id = `detail-${i}-${field}-message`
    return {
      control: { 'aria-invalid': invalidInput || !!problems || undefined, 'aria-describedby': problems ? id : undefined },
      message: problems && <p id={id} className="form-input__message form-input__message--error">{problems.join(' ')}</p>,
    }
  }
  const choose = (i: number, row: DetailRow, field: NewEntityKey, value: string) => {
    if (value === addOption) {
      props.onAddEntity(i, field)
      return
    }
    set(i, row, field, value)
  }
  const fromProduct = (i: number, product: Product) => {
    props.onChange(i, {
      description: product.description || product.title || '',
      price: product.price === undefined || product.price === null ? '' : String(Number(product.price)),
      ...(product.tax_rate_id && rates.some((r) => r.id === product.tax_rate_id) ? { tax_rate_id: String(product.tax_rate_id) } : {}),
      ...(product.ledger_account_id ? { ledger_account_id: String(product.ledger_account_id) } : {}),
      product_id: String(product.id ?? ''),
    })
  }

  return (
    <EditableDocumentForm className="details-editor" label={d('title')}>
      {/* Moneybird's header: the currency, then whether prices include VAT, at the right; their labels are for screen readers. */}
      <div className="editable-document-form__header">
        <div className="details-editor__currency">
          <label className="sr-only" htmlFor={`${hintId}-currency`}>{d('currency')}</label>
          <select
            id={`${hintId}-currency`}
            value={currency}
            disabled={props.currencyLocked}
            aria-invalid={!!errors.field('currency') || undefined}
            onChange={(e) => props.onCurrency(e.target.value)}
          >
            {currencyOptions(t, currency).map((o, i) => <option key={`${o.value}-${i}`} value={o.value} disabled={o.disabled}>{o.label}</option>)}
          </select>
          {errors.field('currency') && <p className="form-input__message form-input__message--error">{errors.field('currency')?.join(' ')}</p>}
        </div>
        <fieldset className="segmented-control" role="radiogroup" aria-labelledby={`${hintId}-prices`}>
          <legend id={`${hintId}-prices`} className="sr-only">{d('pricesAre')}</legend>
          {[false, true].map((incl) => (
            <label key={String(incl)} className={`segmented-control__option${props.pricesInclTax === incl ? ' is-checked' : ''}`}>
              <input type="radio" name={`${hintId}-prices`} value={String(incl)} checked={props.pricesInclTax === incl} onChange={() => props.onPricesInclTax(incl)} />
              {d(incl ? 'inclVat' : 'exclVat')}
            </label>
          ))}
        </fieldset>
      </div>
      <div className="details-editor__body">
        <p id={hintId} className="sr-only">{d('moveHint')}</p>
        <table className="details-editor__table" aria-label={d('title')}>
          <thead>
            <tr>
              <th scope="col" className="details-editor__grip" aria-label={d('order')} />
              <th scope="col" className="details-editor__amount">{d('amount')}</th>
              <th scope="col" className="details-editor__description">{d('description')}</th>
              <th scope="col" className="details-editor__price">{d('priceHeader')}</th>
              <th scope="col" className="details-editor__total">{d('total')}</th>
              <th scope="col" className="details-editor__rate">{d('rateAndCategory')}</th>
              <th scope="col" className="details-editor__remove" aria-label={t('invoices.actions')} />
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => {
              const amount = cell(i, 'amount', invalid[i]?.amount)
              const description = cell(i, 'description')
              const price = cell(i, 'price', invalid[i]?.price)
              const rate = cell(i, 'tax_rate_id')
              const category = cell(i, 'ledger_account_id')
              const period = cell(i, 'period')
              return (
                <tr
                  key={r.key}
                  className={dragging === r.key ? 'is-dragging' : undefined}
                  draggable={dragging === r.key}
                  onDragStart={(e) => {
                    e.dataTransfer.effectAllowed = 'move'
                    e.dataTransfer.setData('text/plain', String(i))
                  }}
                  onDragOver={(e) => {
                    if (dragging) {
                      e.preventDefault()
                    }
                  }}
                  onDrop={(e) => {
                    e.preventDefault()
                    move(Number(e.dataTransfer.getData('text/plain')), i)
                    setDragging(null)
                  }}
                  onDragEnd={() => setDragging(null)}
                >
                  <td className="details-editor__grip">
                    <button
                      type="button"
                      ref={(el) => {
                        if (el) {
                          grips.current.set(r.key, el)
                        } else {
                          grips.current.delete(r.key)
                        }
                      }}
                      className="details-editor__handle"
                      aria-label={d('move', { n: i + 1 })}
                      aria-describedby={hintId}
                      title={d('move', { n: i + 1 })}
                      onMouseDown={() => setDragging(r.key)}
                      onMouseUp={() => setDragging(null)}
                      onKeyDown={onGripKey(i)}
                    >
                      <span aria-hidden="true">⠿</span>
                    </button>
                  </td>
                  <td className="details-editor__amount" data-label={d('amount')}>
                    <input type="text" aria-label={d('amount')} placeholder="#" maxLength={255} value={r.amount} {...amount.control} onChange={(e) => props.onChange(i, { amount: e.target.value })} />
                    {amount.message}
                  </td>
                  <td className="details-editor__description" data-label={d('description')}>
                    <ProductDescription
                      administrationId={administrationId}
                      currency={currency}
                      value={r.description}
                      control={description.control}
                      onChange={(value) => props.onChange(i, { description: value })}
                      onProduct={(p) => fromProduct(i, p)}
                    />
                    {description.message}
                    <PeriodSelect value={r.period} invalid={!!period.message} onChange={(value) => set(i, r, 'period', value)} />
                    {period.message}
                    {applyLink(i, r, 'period')}
                  </td>
                  <td className="details-editor__price" data-label={d('price')}>
                    <input type="text" inputMode="decimal" aria-label={d('price')} placeholder="0.00" value={r.price} {...price.control} onChange={(e) => props.onChange(i, { price: e.target.value })} />
                    {price.message}
                  </td>
                  <td className="details-editor__total" data-label={d('total')} data-testid="detail-total">{money(lineTotals[i] ?? 0)}</td>
                  <td className="details-editor__rate" data-label={d('rateAndCategory')}>
                    <select aria-label={t('invoices.vatRate')} value={r.tax_rate_id} {...rate.control} onChange={(e) => choose(i, r, 'tax_rate_id', e.target.value)}>
                      {rates.map((tr) => <option key={tr.id} value={tr.id}>{tr.name}</option>)}
                      <option value={addOption}>{d('addRate')}</option>
                    </select>
                    {rate.message}
                    {applyLink(i, r, 'tax_rate_id')}
                    <select aria-label={t('invoices.category')} value={r.ledger_account_id} {...category.control} onChange={(e) => choose(i, r, 'ledger_account_id', e.target.value)}>
                      <option value="">{t('invoices.uncategorized')}</option>
                      {groups.map((g) => (
                        <optgroup key={g.type} label={t(`bookkeeping.accountTypes.${g.type}`)}>
                          {g.rows.map(({ account: a, depth }) => (
                            <option key={a.id} value={a.id}>{categoryOption({ account: a, depth })}</option>
                          ))}
                        </optgroup>
                      ))}
                      <option value={addOption}>{d('addCategory')}</option>
                    </select>
                    {category.message}
                    {applyLink(i, r, 'ledger_account_id')}
                  </td>
                  <td className="details-editor__remove">
                    <button type="button" className="details-editor__remove-button" aria-label={d('remove', { n: i + 1 })} title={d('removeRow')} onClick={() => props.onRemove(i)}>
                      <PageIcon kind="close" />
                    </button>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>

        <div className="details-editor__footer">
          {/* "Add detail" and "Discount" joined, as Moneybird's button group. */}
          <div className="btn-group">
            <button type="button" className="btn btn--secondary btn--with-icon" onClick={() => props.onAdd()}>
              <ListIcon kind="plus" />
              <span>{d('add')}</span>
            </button>
            <ActionMenu
              label={t('invoices.discount')}
              items={[
                { label: d('discountDetail'), onSelect: () => props.onAdd({ description: d('discountDescription') }) },
                {
                  label: d('discountTotal'),
                  onSelect: () => {
                    setDiscountShown(true)
                    setTimeout(() => discountInput.current?.focus())
                  },
                },
              ]}
            />
          </div>
          {discountShown && (
            <div className="details-editor__discount">
              <label htmlFor={`${hintId}-discount`}>{t('invoices.discount')}</label>
              <input
                ref={discountInput}
                id={`${hintId}-discount`}
                type="text"
                inputMode="decimal"
                maxLength={10}
                value={discount}
                aria-invalid={discountInvalid || !!errors.field('discount') || undefined}
                onChange={(e) => props.onDiscount(e.target.value)}
              />
              <span>%</span>
              <button
                type="button"
                className="details-editor__remove-button"
                aria-label={d('removeDiscount')}
                title={d('removeDiscount')}
                onClick={() => {
                  props.onDiscount('')
                  setDiscountShown(false)
                }}
              >
                <PageIcon kind="close" />
              </button>
              {errors.field('discount') && <p className="form-input__message form-input__message--error">{errors.field('discount')?.join(' ')}</p>}
            </div>
          )}
          <TotalsTable rows={totals} />
        </div>
      </div>
      <div className="editable-document-form__buttons">
        <button type="button" className="btn btn--primary" disabled={props.saving} onClick={props.onSave}>{t('invoices.editor.save')}</button>
        <button type="button" className="btn btn--secondary" onClick={props.onCancel}>{t('common.cancel')}</button>
      </div>
    </EditableDocumentForm>
  )
}

type DescriptionProps = {
  administrationId: string
  currency: string
  value: string
  control: Record<string, unknown>
  onChange: (value: string) => void
  onProduct: (product: Product) => void
}

const searchDelayMs = 200

/** The description, which searches the products as it is typed and fills the row from the one chosen. */
function ProductDescription({ administrationId, currency, value, control, onChange, onProduct }: DescriptionProps) {
  const { t, i18n } = useTranslation()
  const listId = useId()
  const [open, setOpen] = useState(false)
  const [term, setTerm] = useState('')
  const [active, setActive] = useState(-1)
  const found = useProductSearch(administrationId, term, open)
  const shown = open && term.length >= 2 ? (found.data ?? []) : []

  useEffect(() => {
    const timer = setTimeout(() => setTerm(value.trim()), searchDelayMs)
    return () => clearTimeout(timer)
  }, [value])

  const choose = (product: Product) => {
    onProduct(product)
    setOpen(false)
    setActive(-1)
  }
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (shown.length === 0) {
      return
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => (e.key === 'ArrowDown' ? Math.min(a + 1, shown.length - 1) : Math.max(a - 1, 0)))
    } else if (e.key === 'Enter' && active >= 0) {
      e.preventDefault()
      const product = shown[active]
      if (product) {
        choose(product)
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setOpen(false)
    }
  }

  return (
    <div className="details-editor__product">
      <textarea
        rows={3}
        maxLength={10000}
        role="combobox"
        aria-label={t('invoices.details.description')}
        aria-autocomplete="list"
        aria-expanded={shown.length > 0}
        aria-controls={listId}
        {...(active >= 0 && shown.length > 0 ? { 'aria-activedescendant': `${listId}-${active}` } : {})}
        {...control}
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
          setOpen(true)
          setActive(-1)
        }}
        onKeyDown={onKeyDown}
        onBlur={() => setOpen(false)}
      />
      {shown.length > 0 && (
        <ul id={listId} role="listbox" className="details-editor__products" aria-label={t('invoices.details.products')}>
          {shown.map((p, i) => (
            <li
              key={p.id}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === active}
              className={i === active ? 'is-active' : undefined}
              // Choosing on mouse down keeps the description from losing focus first, which would close the list.
              onMouseDown={(e) => {
                e.preventDefault()
                choose(p)
              }}
            >
              <strong>{p.title || p.description}</strong>
              {p.title && p.description && <span className="hint">{p.description}</span>}
              {p.price !== undefined && p.price !== null && <span className="details-editor__product-price">{formatAmount(Number(p.price), p.currency || currency, i18n.language)}</span>}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** "Period:" under the description, with Moneybird's presets and a range of your own. */
function PeriodSelect({ value, invalid, onChange }: { value: string; invalid: boolean; onChange: (value: string) => void }) {
  const { t, i18n } = useTranslation()
  const d = (key: string) => t(`invoices.details.${key}`)
  const id = useId()
  const root = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const range = periodRange(value)
  const [from, setFrom] = useState(range?.from ?? '')
  const [to, setTo] = useState(range?.to ?? '')
  const pick = (period: string) => {
    onChange(period)
    setOpen(false)
  }

  useEffect(() => {
    if (!open) {
      return
    }
    const onClick = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('click', onClick)
    return () => document.removeEventListener('click', onClick)
  }, [open])

  return (
    <div
      className="details-editor__period"
      ref={root}
      onKeyDown={(e) => {
        if (e.key === 'Escape' && open) {
          e.preventDefault()
          setOpen(false)
        }
      }}
    >
      <button type="button" className={`details-editor__period-button${invalid ? ' is-invalid' : ''}`} aria-expanded={open} aria-controls={id} onClick={() => setOpen(!open)}>
        <span>{d('period')}:</span> <span className={value ? undefined : 'is-placeholder'}>{periodText(value, i18n.language) || d('notSelected')}</span>
      </button>
      {open && (
        <div id={id} className="details-editor__period-panel" role="group" aria-label={d('period')}>
          <ul>
            {periodPresets.map((p) => (
              <li key={p}><button type="button" className="dropdown__item dropdown__action" onClick={() => pick(presetPeriod(p, today()))}>{d(`presets.${p}`)}</button></li>
            ))}
          </ul>
          <div className="details-editor__period-range">
            <label>
              <span>{d('from')}</span>
              <input type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
            </label>
            <label>
              <span>{d('to')}</span>
              <input type="date" value={to} onChange={(e) => setTo(e.target.value)} />
            </label>
            <button type="button" className="btn btn--secondary btn--small" disabled={!toPeriod(from, to)} onClick={() => pick(toPeriod(from, to))}>{d('apply')}</button>
          </div>
          {value && <button type="button" className="link-button" onClick={() => pick('')}>{d('removePeriod')}</button>}
        </div>
      )}
    </div>
  )
}
