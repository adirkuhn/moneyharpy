import { type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { useCurrentUser } from '../auth/queries'
import { formatAmount } from '../banking/queries'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors } from '../forms/Form'
import { ChevronIcon } from '../layout/icons'
import { ContactHoverCard } from './ContactHoverCard'
import { InvoiceHistory } from './InvoiceHistory'
import { invoiceTitle, type Payment, type SalesInvoice, useDeletePayment } from './queries'

type Props = {
  administrationId: string
  invoice: SalesInvoice
  /** Points the document at the lines of a category, or at none. */
  onHighlight: (ledgerAccountId: string | undefined) => void
}

/**
 * The column beside an invoice, after Moneybird's: a tip, what is paid and
 * still to receive, the invoice's id, contact and categories, and its notes
 * and history.
 */
export function InvoiceSidebar({ administrationId, invoice: inv, onHighlight }: Props) {
  const { t, i18n } = useTranslation()
  const accounts = useLedgerAccounts(administrationId)
  const categoryId = useId()
  const date = (d: string | null | undefined) => (d ? new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' }) : '')
  const usesProducts = (inv.details ?? []).some((d) => !!d.product_id)
  const categories = [...new Set((inv.details ?? []).map((d) => d.ledger_account_id).filter((id): id is string => !!id))]
  const facts: [string, ReactNode][] = [
    [t('invoices.view.invoiceId'), invoiceTitle(inv, t)],
    [t('invoices.view.contact'), inv.contact && <ContactHoverCard administrationId={administrationId} contact={{ ...inv.contact, id: inv.contact_id }} />],
    [t('invoices.nextReminder'), date(inv.next_reminder)],
    [t('invoices.reminderCount'), inv.reminder_count ? String(inv.reminder_count) : ''],
    [t('invoices.markedDubiousOn'), date(inv.marked_dubious_on)],
    [t('invoices.markedUncollectibleOn'), date(inv.marked_uncollectible_on)],
  ]
  return (
    <aside className="invoice-sidebar" aria-label={t('invoices.view.sidebar')}>
      {!usesProducts && <ProductsTip administrationId={administrationId} />}
      <Summary administrationId={administrationId} invoice={inv} />
      <dl className="invoice-sidebar__facts">
        {facts.filter(([, value]) => value).map(([label, value]) => (
          <div key={label} className="invoice-sidebar__fact">
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      {categories.length > 0 && (
        <div className="invoice-sidebar__categories">
          <span id={categoryId} className="invoice-sidebar__label">{t('invoices.view.category')}</span>
          <ul aria-labelledby={categoryId}>
            {categories.map((id) => (
              <li key={id} className="category-chip" tabIndex={0} onMouseEnter={() => onHighlight(id)} onMouseLeave={() => onHighlight(undefined)} onFocus={() => onHighlight(id)} onBlur={() => onHighlight(undefined)}>
                <TagIcon />
                <span>{accounts.data?.find((a) => a.id === id)?.name ?? ''}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
      <InvoiceHistory administrationId={administrationId} invoice={inv} />
    </aside>
  )
}

function TagIcon() {
  return (
    <svg viewBox="0 0 24 24" width="0.875em" height="0.875em" aria-hidden="true" fill="currentColor">
      <path d="M3 4.500A1.500 1.500 0 0 1 4.500 3h6.400a1.500 1.500 0 0 1 1.060.44l8.500 8.500a1.500 1.500 0 0 1 0 2.120l-6.400 6.400a1.500 1.500 0 0 1-2.120 0l-8.500-8.500A1.500 1.500 0 0 1 3 10.900Zm4.500 4.500a1.500 1.500 0 1 0 0-3 1.500 1.500 0 0 0 0 3Z" />
    </svg>
  )
}

const tipKey = (userId: string) => `moneyharpy.tips.sales-invoice-products.${userId}`

function dismissed(userId: string) {
  try {
    return localStorage.getItem(tipKey(userId)) === 'dismissed'
  } catch {
    return false
  }
}

/** Moneybird's card about products; once closed it stays away for this user. */
function ProductsTip({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const me = useCurrentUser()
  const userId = String(me.data?.id ?? 'anonymous')
  const [closed, setClosed] = useState<string | null>(null)
  const heading = useId()
  if (!me.data || closed === userId || dismissed(userId)) {
    return null
  }
  const close = () => {
    try {
      localStorage.setItem(tipKey(userId), 'dismissed')
    } catch {
      // Without storage the card only stays closed on this page.
    }
    setClosed(userId)
  }
  return (
    <section className="feature-card" aria-labelledby={heading}>
      <h2 id={heading} className="feature-card__title">{t('invoices.view.tip.title')}</h2>
      <p className="feature-card__text">{t('invoices.view.tip.text')}</p>
      <Link className="btn btn--on-color" to={`/${administrationId}/products/new`}>{t('invoices.view.tip.action')}</Link>
      <button type="button" className="feature-card__close" aria-label={t('invoices.view.tip.close')} title={t('invoices.view.tip.close')} onClick={close}>
        <svg viewBox="0 0 24 24" width="1em" height="1em" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round"><path d="M6 6l12 12M18 6 6 18" /></svg>
      </button>
      <span className="feature-card__art" aria-hidden="true" />
    </section>
  )
}

/** Total, the payments registered and what is still to receive, after Moneybird's box. */
function Summary({ administrationId, invoice: inv }: { administrationId: string; invoice: SalesInvoice }) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const listId = useId()
  const remove = useDeletePayment(administrationId, inv.id ?? '')
  const money = (v: string | number | null | undefined) => formatAmount(v, inv.currency ?? 'EUR', i18n.language)
  const date = (d: string | null | undefined) => (d ? new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' }) : '')
  const payments = (inv.payments ?? []) as Payment[]
  const method = (p: Payment) => {
    if (p.financial_mutation_id) {
      return <Link to={`/${administrationId}/financial_mutations/${p.financial_mutation_id}`}>{t('invoices.payments.bankTransaction')}</Link>
    }
    return t(`invoices.payments.actions.${p.manual_payment_action ?? 'bank_transfer'}`, { defaultValue: p.manual_payment_action ?? '' })
  }
  return (
    <>
      <Problems messages={formErrors(remove.error, {}).general} />
      <ul className="invoice-summary" aria-label={t('invoices.view.summary')}>
        <li className="invoice-summary__item invoice-summary__item--total">
          <span>{t('invoices.view.total')}</span>
          <span>{money(inv.total_price_incl_tax)}</span>
        </li>
        {payments.length > 0 && (
          <li className="invoice-summary__payments">
            <button type="button" className="invoice-summary__item invoice-summary__toggle" aria-expanded={open} aria-controls={listId} onClick={() => setOpen(!open)}>
              <span>{t('invoices.view.viewPayments', { count: payments.length })}</span>
              <ChevronIcon className={open ? 'is-open' : undefined} />
            </button>
            {open && (
              <ul id={listId} className="invoice-summary__list" aria-label={t('invoices.view.registered')}>
                {payments.map((p) => (
                  <li key={p.id} className="invoice-summary__payment">
                    <span className="invoice-summary__payment-title">
                      <span>{date(p.payment_date)}</span>
                      <span>{money(p.price)}</span>
                    </span>
                    <span className="invoice-summary__payment-meta">
                      <span>{method(p)}</span>
                      {inv.state !== 'uncollectible' && (
                        <ConfirmDelete
                          label={t('invoices.payments.remove', { date: date(p.payment_date) })}
                          confirmLabel={t('invoices.payments.confirmRemove', { date: date(p.payment_date) })}
                          text={p.financial_mutation_id ? t('invoices.payments.unlink') : t('invoices.view.notes.delete')}
                          confirmText={t('contacts.confirmRemove')}
                          disabled={remove.isPending}
                          onConfirm={() => remove.mutate(p.id ?? '')}
                        />
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </li>
        )}
        <li className="invoice-summary__item">
          <span>{t('invoices.view.toReceive')}</span>
          <span>{money(inv.total_unpaid)}</span>
        </li>
      </ul>
    </>
  )
}
