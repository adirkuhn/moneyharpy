import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { type SalesInvoice, type Sending, useSendSalesInvoice } from './queries'

const methods = ['Email', 'Post', 'Manual'] as const
type Method = (typeof methods)[number]

function tomorrow() {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  return d.toISOString().slice(0, 10)
}

/** How an invoice goes out: by email with a message, by post or by the user; now or on a later date. */
export function SendPanel({ administrationId, invoice, onDone }: { administrationId: string; invoice: SalesInvoice; onDone: () => void }) {
  const { t } = useTranslation()
  const send = useSendSalesInvoice(administrationId, invoice.id ?? '')
  const known = [invoice.delivery_method, invoice.contact?.delivery_method].find((m): m is Method => (methods as readonly string[]).includes(m ?? ''))
  const [method, setMethod] = useState<Method>(known ?? 'Email')
  const [email, setEmail] = useState(invoice.contact?.send_invoices_to_email ?? '')
  const [message, setMessage] = useState('')
  const [ubl, setUbl] = useState(invoice.contact?.email_ubl ?? false)
  const [scheduled, setScheduled] = useState(false)
  const [date, setDate] = useState(tomorrow)
  const draft = invoice.state === 'draft'
  const labels = { delivery_method: t('invoices.sendBy'), email_address: t('invoices.emailAddress'), email_message: t('invoices.message'), invoice_date: t('invoices.sendOn') }
  const errors = formErrors(send.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: Sending = {
      delivery_method: method,
      ...(method === 'Email' ? { email_address: email.trim(), email_message: message, deliver_ubl: ubl } : {}),
      ...(draft && scheduled ? { sending_scheduled: true, invoice_date: date } : {}),
    }
    send.mutate(body, { onSuccess: onDone })
  }

  return (
    <section className="settings-block send-panel" aria-labelledby="send-invoice">
      <div className="settings-block__header">
        <h2 id="send-invoice">{t('invoices.sendTitle')}</h2>
      </div>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <SelectInput label={labels.delivery_method} width="medium" value={method} errors={errors.field('delivery_method')} onChange={(e) => setMethod(e.target.value as Method)}>
          {methods.map((m) => <option key={m} value={m}>{t(`contacts.delivery.${m}`)}</option>)}
        </SelectInput>
        {method === 'Email' && (
          <>
            <TextInput label={labels.email_address} type="email" maxLength={1000} help={t('contacts.emailHelp')} value={email} errors={errors.field('email_address')} onChange={(e) => setEmail(e.target.value)} />
            <TextArea label={labels.email_message} rows={5} maxLength={10000} placeholder={t('invoices.messagePlaceholder')} help={t('invoices.messageHelp')} value={message} errors={errors.field('email_message')} onChange={(e) => setMessage(e.target.value)} />
            <CheckboxInput label={t('contacts.emailUBL')} checked={ubl} onChange={(e) => setUbl(e.target.checked)} />
          </>
        )}
        {method !== 'Email' && <p className="hint send-panel__hint">{t(method === 'Post' ? 'invoices.postHint' : 'invoices.manualHint')}</p>}
        {draft && <CheckboxInput label={t('invoices.schedule')} checked={scheduled} onChange={(e) => setScheduled(e.target.checked)} />}
        {draft && scheduled && (
          <TextInput label={labels.invoice_date} type="date" width="medium" min={tomorrow()} value={date} help={t('invoices.scheduleHelp')} errors={errors.field('invoice_date')} onChange={(e) => setDate(e.target.value)} />
        )}
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={send.isPending}>{draft && scheduled ? t('invoices.scheduleSubmit') : t('invoices.sendSubmit')}</button>
          <button type="button" className="btn btn--secondary" onClick={onDone}>{t('common.cancel')}</button>
        </div>
      </form>
    </section>
  )
}
