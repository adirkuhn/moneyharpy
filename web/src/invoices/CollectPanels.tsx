import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { DateInput } from '../forms/DatePicker'
import { formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { type Collectable, type InvoiceKind, remindersFor, type SalesInvoice, today, useSendReminders, useWriteOff, type WriteOff } from './queries'

type Props = { administrationId: string; invoice: SalesInvoice; onDone: () => void }

type WriteOffProps = { administrationId: string; invoice: Collectable; kind: WriteOff['kind']; invoiceKind?: InvoiceKind; onDone: () => void }

/** Marks an invoice dubious, or writes it off for good as uncollectible, on a date. */
export function WriteOffPanel({ administrationId, invoice, kind, invoiceKind = 'sales', onDone }: WriteOffProps) {
  const { t } = useTranslation()
  const writeOff = useWriteOff(administrationId, invoice.id ?? '', invoiceKind)
  const [date, setDate] = useState(today)
  const field = kind === 'dubious' ? 'dubious_date' : 'uncollectible_date'
  const errors = formErrors(writeOff.error, { [field]: t('invoices.writeOff.date') })
  const title = t(`invoices.writeOff.${kind}Title`)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    writeOff.mutate({ kind, date }, { onSuccess: onDone })
  }
  return (
    <section className="settings-block send-panel" aria-label={title}>
      <div className="settings-block__header">
        <h2>{title}</h2>
      </div>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <p className="hint send-panel__hint">{t(`invoices.writeOff.${kind}Hint`)}</p>
        <DateInput label={t('invoices.writeOff.date')} width="medium" max={today()} required value={date} errors={errors.field(field)} onChange={setDate} />
        <div className="form-sheet__actions">
          <button className={kind === 'uncollectible' ? 'btn btn--danger' : 'btn btn--primary'} disabled={writeOff.isPending || !date}>{t(`invoices.writeOff.${kind}Submit`)}</button>
          <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
        </div>
      </form>
    </section>
  )
}

const methods = ['Email', 'Manual'] as const
type Method = (typeof methods)[number]

/** Sends a reminder about an invoice by email, or marks it reminded when the user sends it. */
export function ReminderPanel({ administrationId, invoice, onDone }: Props) {
  const { t } = useTranslation()
  const send = useSendReminders(administrationId)
  const [method, setMethod] = useState<Method>(invoice.contact?.delivery_method === 'Email' || !invoice.contact?.delivery_method ? 'Email' : 'Manual')
  const [email, setEmail] = useState(invoice.contact?.send_invoices_to_email ?? '')
  const [text, setText] = useState('')
  const labels = { delivery_method: t('invoices.sendBy'), email_address: t('invoices.emailAddress'), reminder_text: t('invoices.reminders.text') }
  const errors = formErrors(send.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const extra = { delivery_method: method, ...(method === 'Email' ? { email_address: email.trim() } : {}), ...(text.trim() ? { reminder_text: text } : {}) }
    send.mutate(remindersFor([invoice], extra), { onSuccess: onDone })
  }
  return (
    <section className="settings-block send-panel" aria-label={t('invoices.reminders.title')}>
      <div className="settings-block__header">
        <h2>{t('invoices.reminders.title')}</h2>
      </div>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <SelectInput label={labels.delivery_method} width="medium" value={method} onChange={(e) => setMethod(e.target.value as Method)}>
          {methods.map((m) => <option key={m} value={m}>{t(`contacts.delivery.${m}`)}</option>)}
        </SelectInput>
        {method === 'Email' ? (
          <TextInput label={labels.email_address} type="email" maxLength={1000} value={email} errors={errors.field('email_address')} onChange={(e) => setEmail(e.target.value)} />
        ) : (
          <p className="hint send-panel__hint">{t('invoices.reminders.manualHint')}</p>
        )}
        <TextArea label={labels.reminder_text} rows={5} maxLength={10000} placeholder={t('invoices.reminders.textPlaceholder')} help={t('invoices.reminders.textHelp')} value={text} errors={errors.field('reminder_text')} onChange={(e) => setText(e.target.value)} />
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={send.isPending}>{t('invoices.reminders.submit')}</button>
          <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
        </div>
      </form>
    </section>
  )
}
