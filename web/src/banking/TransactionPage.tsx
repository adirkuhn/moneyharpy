import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { FormSection, formErrors, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { Block } from '../settings/Block'
import { CategorySelect } from './CategorySelect'
import { type Booking, formatAmount, type Mutation, useFinancialAccounts, useLinkBooking, useMutationOne, useUnlinkBooking } from './queries'

/** One transaction: its details, its bookings and a form to book what is open. */
export function TransactionPage() {
  const { administrationId = '', mutationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const mutation = useMutationOne(administrationId, mutationId)
  if (mutation.error instanceof ApiError && mutation.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data || !mutation.data) {
    return <Loading />
  }
  return <Transaction administrationId={administrationId} language={administration.data.language} mutation={mutation.data} />
}

function Transaction({ administrationId, mutation: m }: { administrationId: string; language: string; mutation: Mutation }) {
  const { t, i18n } = useTranslation()
  const accounts = useFinancialAccounts(administrationId)
  const categories = useLedgerAccounts(administrationId)
  const unlink = useUnlinkBooking(administrationId, m.id ?? '')
  const money = (v: string | null | undefined) => formatAmount(v, m.currency ?? 'EUR', i18n.language)
  const names = new Map(categories.data?.map((c) => [c.id, c.name]))
  const account = accounts.data?.find((a) => a.id === m.financial_account_id)
  const open = Number(m.amount_open ?? 0)
  const back = `/${administrationId}/financial_mutations`

  return (
    <Page title={m.contra_account_name || m.message || t('bank.transaction')} subtitle={money(m.amount)} back={{ to: back, label: t('bank.toTransactions') }}>
      <dl className="details card">
        <div><dt>{t('bank.date')}</dt><dd>{new Date(`${m.date}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'long' })}</dd></div>
        <div><dt>{t('bank.account')}</dt><dd>{account?.name ?? '-'}</dd></div>
        <div><dt>{t('bank.amount')}</dt><dd className={Number(m.amount) < 0 ? 'is-out' : 'is-in'}>{money(m.amount)}</dd></div>
        {m.contra_account_name && <div><dt>{t('bank.contraName')}</dt><dd>{m.contra_account_name}</dd></div>}
        {m.contra_account_number && <div><dt>{t('bank.contraAccount')}</dt><dd>{m.contra_account_number}</dd></div>}
        {m.message && <div><dt>{t('bank.description')}</dt><dd>{m.message}</dd></div>}
        <div><dt>{t('bank.open')}</dt><dd>{m.state === 'processed' ? t('bank.booked') : money(m.amount_open)}</dd></div>
      </dl>

      <Block id="bookings" title={t('bank.bookings')} subtitle={t('bank.bookingsSubtitle')}>
        <Problems messages={formErrors(unlink.error, {}).general} />
        {m.ledger_account_bookings.length === 0 ? (
          <p className="empty">{t('bank.noBookings')}</p>
        ) : (
          <table className="data-table" aria-label={t('bank.bookings')}>
            <thead>
              <tr>
                <th scope="col">{t('bank.category')}</th>
                <th scope="col" className="transactions__amount">{t('bank.amount')}</th>
                <th scope="col"><span className="sr-only">{t('bookkeeping.actions')}</span></th>
              </tr>
            </thead>
            <tbody>
              {(m.ledger_account_bookings as Booking[]).map((b) => (
                <tr key={b.id}>
                  <td>
                    {names.get(b.ledger_account_id ?? '') ?? b.ledger_account_id}
                    {b.description && <p className="hint">{b.description}</p>}
                    {b.booking_rule_id && <p className="hint">{t('bank.byRule')}</p>}
                  </td>
                  <td className="transactions__amount">{money(b.price)}</td>
                  <td className="data-table__actions">
                    <button type="button" className="btn btn--secondary btn--small" disabled={unlink.isPending} aria-label={t('bank.removeBookingOn', { name: names.get(b.ledger_account_id ?? '') ?? '' })} onClick={() => unlink.mutate(b.id ?? '')}>
                      {t('bank.removeBooking')}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Block>

      {open !== 0 && <BookForm key={m.amount_open} administrationId={administrationId} mutation={m} />}
    </Page>
  )
}

function BookForm({ administrationId, mutation: m }: { administrationId: string; mutation: Mutation }) {
  const { t } = useTranslation()
  const link = useLinkBooking(administrationId, m.id ?? '')
  // Amounts are entered in the transaction's direction: positive books it.
  const rest = String(Math.abs(Number(m.amount_open ?? 0)))
  const [form, setForm] = useState({ booking_id: '', price_base: rest, description: '' })
  const labels = { booking_id: t('bank.category'), price_base: t('bank.amount'), description: t('bank.description'), date: t('bank.date') }
  const errors = formErrors(link.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const price = Number(form.price_base.replace(',', '.'))
    const sign = Number(m.amount) < 0 === Number(m.amount_open) < 0 ? 1 : -1
    link.mutate({ ...form, price_base: Number.isFinite(price) ? String(price * sign) : form.price_base })
  }
  return (
    <Block id="book" title={t('bank.book')} subtitle={t('bank.bookSubtitle')}>
      <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate aria-label={t('bank.book')}>
        <Problems messages={errors.general} />
        <FormSection>
          <CategorySelect administrationId={administrationId} label={labels.booking_id} value={form.booking_id} errors={errors.field('booking_id')} onChange={(booking_id) => setForm({ ...form, booking_id })} />
          <TextInput label={labels.price_base} inputMode="decimal" width="small" maxLength={16} help={t('bank.splitHelp')} value={form.price_base} errors={errors.field('price_base') ?? errors.field('date')} onChange={(e) => setForm({ ...form, price_base: e.target.value })} />
          <TextInput label={labels.description} maxLength={1000} value={form.description} errors={errors.field('description')} onChange={(e) => setForm({ ...form, description: e.target.value })} />
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={link.isPending || !form.booking_id}>{t('bank.bookButton')}</button>
        </div>
      </form>
    </Block>
  )
}
