import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import {
  type Collectable, type InvoiceKind, isPayable, type Payment, type PaymentAction, paymentActions, today, useDeletePayment, useRegisterPayment, useSettleCredit,
} from './queries'

type Props = { administrationId: string; invoice: Collectable; kind?: InvoiceKind }

/** What was paid on an invoice, and registering a payment made by hand. */
export function PaymentsPanel({ administrationId, invoice: inv, kind = 'sales' }: Props) {
  const { t, i18n } = useTranslation()
  const remove = useDeletePayment(administrationId, inv.id ?? '', kind)
  const settle = useSettleCredit(administrationId, inv.id ?? '')
  const money = (v: string | number | null | undefined) => formatAmount(v, inv.currency ?? 'EUR', i18n.language)
  const date = (d: string | null | undefined) => (d ? new Date(`${d}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'medium' }) : '')
  const payments = (inv.payments ?? []) as Payment[]
  const payable = isPayable(inv.state)
  const credit = payable && !!inv.original_sales_invoice_id && Number(inv.total_price_incl_tax ?? 0) < 0
  const problems = formErrors(remove.error ?? settle.error, {}).general

  const method = (p: Payment) => {
    if (p.financial_mutation_id) {
      return <Link to={`/${administrationId}/financial_mutations/${p.financial_mutation_id}`}>{t('invoices.payments.bankTransaction')}</Link>
    }
    if (p.manual_payment_action) {
      return t(`invoices.payments.actions.${p.manual_payment_action}`, { defaultValue: p.manual_payment_action })
    }
    return t('invoices.payments.actions.bank_transfer')
  }

  return (
    <section className="settings-block" aria-labelledby="invoice-payments">
      <div className="settings-block__header">
        <h2 id="invoice-payments">{t('invoices.payments.title')}</h2>
      </div>
      <Problems messages={problems} />
      {payments.length === 0 ? (
        <p className="empty">{t('invoices.payments.none')}</p>
      ) : (
        <table className="data-table" aria-label={t('invoices.payments.title')}>
          <thead>
            <tr>
              <th scope="col">{t('invoices.payments.date')}</th>
              <th scope="col">{t('invoices.payments.method')}</th>
              <th scope="col" className="invoice-view__money">{t('invoices.amount')}</th>
              <th scope="col"><span className="sr-only">{t('invoices.actions')}</span></th>
            </tr>
          </thead>
          <tbody>
            {payments.map((p) => (
              <tr key={p.id}>
                <td>{date(p.payment_date)}</td>
                <td>{method(p)}</td>
                <td className="invoice-view__money">{money(p.price)}</td>
                <td className="data-table__actions">
                  {inv.state !== 'uncollectible' && (
                    <ConfirmDelete
                      label={t('invoices.payments.remove', { date: date(p.payment_date) })}
                      confirmLabel={t('invoices.payments.confirmRemove', { date: date(p.payment_date) })}
                      text={p.financial_mutation_id ? t('invoices.payments.unlink') : t('contacts.remove')}
                      confirmText={t('contacts.confirmRemove')}
                      disabled={remove.isPending}
                      onConfirm={() => remove.mutate(p.id ?? '')}
                    />
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {credit && (
        <div className="form-sheet__actions">
          <button type="button" className="btn btn--secondary" disabled={settle.isPending} onClick={() => settle.mutate()}>{t('invoices.payments.settleCredit')}</button>
          <span className="hint">{t('invoices.payments.settleCreditHelp')}</span>
        </div>
      )}
      {payable && !credit && <PaymentForm key={inv.total_unpaid} administrationId={administrationId} invoice={inv} kind={kind} />}
    </section>
  )
}

function PaymentForm({ administrationId, invoice: inv, kind }: Props) {
  const { t } = useTranslation()
  const register = useRegisterPayment(administrationId, inv.id ?? '', kind)
  const [form, setForm] = useState({ payment_date: today(), price: String(Number(inv.total_unpaid ?? 0)), manual_payment_action: 'bank_transfer' as PaymentAction })
  const labels = { payment_date: t('invoices.payments.date'), price: t('invoices.amount'), manual_payment_action: t('invoices.payments.method') }
  const errors = formErrors(register.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    register.mutate({ ...form, price: form.price.trim().replace(',', '.') })
  }
  return (
    <form className="form-sheet" onSubmit={submit} noValidate aria-label={t('invoices.payments.register')}>
      <Problems messages={errors.general} />
      <FormSection title={t('invoices.payments.register')}>
        <TextInput label={labels.payment_date} type="date" width="medium" max={today()} required value={form.payment_date} errors={errors.field('payment_date')} onChange={(e) => setForm({ ...form, payment_date: e.target.value })} />
        <TextInput label={labels.price} inputMode="decimal" width="small" maxLength={16} required help={t('invoices.payments.amountHelp')} value={form.price} errors={errors.field('price')} onChange={(e) => setForm({ ...form, price: e.target.value })} />
        <SelectInput label={labels.manual_payment_action} width="medium" value={form.manual_payment_action} errors={errors.field('manual_payment_action')} onChange={(e) => setForm({ ...form, manual_payment_action: e.target.value as PaymentAction })}>
          {paymentActions.map((a) => <option key={a} value={a}>{t(`invoices.payments.actions.${a}`)}</option>)}
        </SelectInput>
      </FormSection>
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={register.isPending || !form.price.trim() || !form.payment_date}>{t('invoices.payments.registerButton')}</button>
      </div>
    </form>
  )
}
