import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { DateInput } from '../forms/DatePicker'
import { formErrors, FormRow, TextArea } from '../forms/Form'
import { useInvoiceSettings, useWorkflows } from '../invoicing/queries'
import { Tip } from './AddInvoiceDialog'
import { invoiceTitle, type SalesInvoice, type Sending, useSendSalesInvoice } from './queries'

/** The ways Moneybird's send form offers, in its order. */
const methods = ['Email', 'Peppol', 'Manual'] as const
type Method = (typeof methods)[number]
const available = (m: Method) => m !== 'Peppol'

/** Tomorrow in the browser's time zone, as yyyy-mm-dd: the first day sending can wait for. */
export function tomorrow() {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** How the invoice goes out unless asked otherwise: as it went before, else as its contact prefers. */
function preferredMethod(inv: SalesInvoice): string {
  return inv.delivery_method || inv.contact?.delivery_method || 'Email'
}

type PanelProps = { administrationId: string; invoice: SalesInvoice; onDone: () => void }

/** A form above the document, after Moneybird's send forms: a title, the fields, and the buttons centred underneath. */
function SendForm({ title, intro, errors, pending, onSubmit, onCancel, children }: {
  title: string; intro?: ReactNode; errors: string[]; pending: boolean; onSubmit: (e: FormEvent) => void; onCancel: () => void; children?: ReactNode
}) {
  const { t } = useTranslation()
  const heading = useId()
  return (
    <section className="send-form" aria-labelledby={heading}>
      <form onSubmit={onSubmit} noValidate>
        <div className="send-form__intro">
          <h3 id={heading} className="send-form__title">{title}</h3>
          {intro}
        </div>
        <Problems messages={errors} />
        {children && <div className={`send-form__fields${intro ? ' send-form__fields--divided' : ''}`}>{children}</div>}
        <div className="send-form__actions">
          <button className="btn btn--primary" disabled={pending}>{t('invoices.sendForm.submit')}</button>
          <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
        </div>
      </form>
    </section>
  )
}

/**
 * Moneybird's "Send invoice": how it goes out as cards, then for email the
 * addresses, the message with its placeholders and the UBL invoice.
 */
export function SendPanel({ administrationId, invoice: inv, onDone }: PanelProps) {
  const { t, i18n } = useTranslation()
  const send = useSendSalesInvoice(administrationId, inv.id ?? '')
  const workflows = useWorkflows(administrationId)
  const settings = useInvoiceSettings(administrationId)
  const preferred = preferredMethod(inv)
  const [method, setMethod] = useState<Method>((methods as readonly string[]).includes(preferred) && available(preferred as Method) ? (preferred as Method) : 'Email')
  const [emails, setEmails] = useState<string[]>(() => {
    const known = (inv.contact?.send_invoices_to_email ?? '').split(',').map((e) => e.trim()).filter(Boolean)
    return known.length > 0 ? known : ['']
  })
  const [message, setMessage] = useState<string | null>(null)
  const [ubl, setUbl] = useState<boolean | null>(null)

  // The message starts as the workflow's email text, or the default one, in the invoice's language.
  const flow = (workflows.data ?? []).find((w) => w.id === inv.workflow_id) ?? (workflows.data ?? []).find((w) => w.type === 'InvoiceWorkflow' && w.default)
  const dt = i18n.getFixedT(inv.language || flow?.language || i18n.language)
  const text = message ?? (flow?.invoice_email_body?.trim() || dt('invoices.sendForm.defaultMessage'))
  const attachUbl = ubl ?? (!!inv.contact?.email_ubl || !!settings.data?.default_email_ubl)

  const labels = {
    delivery_method: t('invoices.sendForm.method'), email_address: t('invoices.sendForm.emailAddress'), email_message: t('invoices.sendForm.message'),
    deliver_ubl: t('invoices.sendForm.ubl'),
  }
  const errors = formErrors(send.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: Sending = {
      delivery_method: method,
      ...(method === 'Email'
        ? { email_address: emails.map((m) => m.trim()).filter(Boolean).join(', '), email_message: text, deliver_ubl: attachUbl }
        : {}),
    }
    send.mutate(body, { onSuccess: onDone })
  }
  const changeEmail = (i: number, value: string) => setEmails(emails.map((m, j) => (j === i ? value : m)))

  return (
    <SendForm
      title={t('invoices.sendForm.title', { name: invoiceTitle(inv, t) })}
      errors={errors.general}
      pending={send.isPending}
      onSubmit={submit}
      onCancel={onDone}
    >
      <div className="send-methods" role="radiogroup" aria-label={labels.delivery_method}>
        {methods.map((m) => (
          <label key={m} className={`send-methods__card${m === method ? ' is-checked' : ''}${available(m) ? '' : ' is-disabled'}`}>
            <span className="send-methods__header">
              <span className="send-methods__title">{t(`invoices.sendForm.methods.${m}`)}</span>
              <input type="radio" name="delivery_method" value={m} checked={m === method} disabled={!available(m)} onChange={() => setMethod(m)} />
            </span>
            <span className="send-methods__text">{t(`invoices.sendForm.methodsText.${m}`)}</span>
            {!available(m) && <span className="send-methods__note">{t('list.notAvailable')}</span>}
          </label>
        ))}
      </div>
      {errors.field('delivery_method') && <p className="form-input__message form-input__message--error">{errors.field('delivery_method')?.join(' ')}</p>}
      {method === 'Email' && (
        <div className="send-form__section">
          <FormRow label={labels.email_address} errors={errors.field('email_address')}>
            {(control) => (
              <div className="send-form__emails">
                {emails.map((value, i) => (
                  <div key={i} className="send-form__email">
                    <input
                      type="email"
                      maxLength={1000}
                      value={value}
                      onChange={(e) => changeEmail(i, e.target.value)}
                      {...(i === 0
                        ? { id: control.id, 'aria-invalid': control['aria-invalid'], 'aria-describedby': control['aria-describedby'] }
                        : { 'aria-label': t('invoices.sendForm.emailAddressN', { n: i + 1 }) })}
                    />
                    {i === 0 ? (
                      <button type="button" className="send-form__icon-button" aria-label={t('invoices.sendForm.addEmail')} title={t('invoices.sendForm.addEmail')} onClick={() => setEmails([...emails, ''])}>
                        <span aria-hidden="true">+</span>
                      </button>
                    ) : (
                      <button
                        type="button"
                        className="send-form__icon-button"
                        aria-label={t('invoices.sendForm.removeEmail', { n: i + 1 })}
                        title={t('invoices.sendForm.removeEmail', { n: i + 1 })}
                        onClick={() => setEmails(emails.filter((_, j) => j !== i))}
                      >
                        <span aria-hidden="true">−</span>
                      </button>
                    )}
                  </div>
                ))}
              </div>
            )}
          </FormRow>
          <TextArea
            label={labels.email_message}
            rows={8}
            maxLength={10000}
            help={t('invoices.sendForm.messageHelp')}
            value={text}
            errors={errors.field('email_message')}
            onChange={(e) => setMessage(e.target.value)}
          />
          <FormRow label={labels.deliver_ubl} labelExtra={<Tip text={t('invoices.sendForm.ublTip')} />} errors={errors.field('deliver_ubl')}>
            {({ labelId: _, ...control }) => (
              <label className="send-form__switch">
                <input type="checkbox" role="switch" className="switch" {...control} checked={attachUbl} onChange={(e) => setUbl(e.target.checked)} />
                <span>{t('invoices.sendForm.attachUbl')}</span>
              </label>
            )}
          </FormRow>
        </div>
      )}
    </SendForm>
  )
}

/**
 * Moneybird's "Send invoice in the future": the date the invoice is sent on,
 * by the contact's own delivery method, and whether it merges with others.
 */
export function SchedulePanel({ administrationId, invoice: inv, onDone }: PanelProps) {
  const { t } = useTranslation()
  const send = useSendSalesInvoice(administrationId, inv.id ?? '')
  const [date, setDate] = useState(() => (inv.state === 'scheduled' && inv.invoice_date && inv.invoice_date >= tomorrow() ? inv.invoice_date : tomorrow()))
  const [mergeable, setMergeable] = useState(false)
  const labels = { invoice_date: t('invoices.scheduleForm.date'), mergeable: t('invoices.scheduleForm.mergeable') }
  const errors = formErrors(send.error, labels)
  const method = preferredMethod(inv)
  const email = inv.contact?.send_invoices_to_email
  const known = ['Email', 'Manual', 'Post', 'Peppol', 'Simplerinvoicing'].includes(method)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    send.mutate({ sending_scheduled: true, invoice_date: date, mergeable }, { onSuccess: onDone })
  }

  return (
    <SendForm
      title={t('invoices.scheduleForm.title')}
      intro={
        <p className="send-form__text">
          {t('invoices.scheduleForm.intro')}{' '}
          {known && (
            <>
              {t('invoices.scheduleForm.prefers')} <strong>{t(`invoices.scheduleForm.methods.${method}`)}</strong>
              {method === 'Email' && email ? ` ${t('invoices.scheduleForm.at', { email })}` : '.'}
            </>
          )}
        </p>
      }
      errors={errors.general}
      pending={send.isPending}
      onSubmit={submit}
      onCancel={onDone}
    >
      <DateInput label={labels.invoice_date} width="medium" min={tomorrow()} value={date} errors={errors.field('invoice_date')} onChange={setDate} />
      <div className="form-input form-input--check">
        <span className="form-input__label" aria-hidden="true" />
        <div className="form-input__body">
          <label className="check">
            <input type="checkbox" checked={mergeable} onChange={(e) => setMergeable(e.target.checked)} />
            <span>{labels.mergeable}</span>
          </label>
        </div>
      </div>
    </SendForm>
  )
}
