import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { FormSection, formErrors, TextInput } from '../forms/Form'
import { Block } from '../settings/Block'
import { type InvoiceSettings, useUpdateInvoiceSettings } from './queries'

/** Addresses that get a copy of every invoice, quote or order. */
export function ForwardingBlock({ administrationId, settings }: { administrationId: string; settings: InvoiceSettings }) {
  const { t } = useTranslation()
  const saved = {
    invoice_bcc_email_address: settings.invoice_bcc_email_address,
    invoice_reminders_bcc_enabled: settings.invoice_reminders_bcc_enabled,
    estimate_bcc_email_address: settings.estimate_bcc_email_address,
    order_bcc_email_address: settings.order_bcc_email_address,
  }
  const [form, setForm] = useState(saved)
  const update = useUpdateInvoiceSettings(administrationId)
  const dirty = JSON.stringify(form) !== JSON.stringify(saved)
  const labels = {
    invoice_bcc_email_address: t('invoicing.invoices'), estimate_bcc_email_address: t('invoicing.quotes'), order_bcc_email_address: t('invoicing.checkout'),
  }
  const errors = formErrors(update.error, labels)
  const field = (key: keyof typeof labels, placeholder: string) => (
    <TextInput
      label={labels[key]}
      type="text"
      inputMode="email"
      autoComplete="off"
      placeholder={placeholder}
      value={form[key]}
      errors={errors.field(key)}
      onChange={(e) => setForm({ ...form, [key]: e.target.value })}
    />
  )
  const noInvoiceAddress = form.invoice_bcc_email_address.trim() === ''
  const submit = (e: FormEvent) => {
    e.preventDefault()
    update.mutate(form)
  }

  return (
    <Block id="forward-documents" title={t('invoicing.parts.forwarding')} subtitle={t('invoicing.forwardingSubtitle')}>
      <form className="card card--with-footer forwarding" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          {field('invoice_bcc_email_address', 'invoices@example.org')}
          <div className="form-input form-input--check">
            <span className="form-input__label">{t('invoicing.invoiceReminders')}</span>
            <div className="form-input__body" title={noInvoiceAddress ? t('invoicing.remindersFirst') : undefined}>
              <label className="check">
                <input
                  type="checkbox"
                  checked={form.invoice_reminders_bcc_enabled && !noInvoiceAddress}
                  disabled={noInvoiceAddress}
                  onChange={(e) => setForm({ ...form, invoice_reminders_bcc_enabled: e.target.checked })}
                />
                <span>{t('invoicing.reminders')}</span>
              </label>
            </div>
          </div>
        </FormSection>
        <FormSection>{field('estimate_bcc_email_address', 'quotes@example.org')}</FormSection>
        <FormSection>{field('order_bcc_email_address', 'orders@example.org')}</FormSection>
        {dirty && (
          <div className="forwarding__actions">
            <button className="btn btn--primary" disabled={update.isPending}>{t('invoicing.save')}</button>
            <button type="button" className="btn btn--secondary" onClick={() => setForm(saved)}>{t('invoicing.cancel')}</button>
          </div>
        )}
        <p className="card__footer">{t('invoicing.forwardingHelp')}</p>
      </form>
    </Block>
  )
}
