import { type ReactNode, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { formatAmount } from '../banking/queries'
import { useWorkflows } from '../invoicing/queries'
import { formatIBAN } from '../settings/format'
import { ContactIcon, type ContactIconKind } from './ContactIcon'
import type { Contact } from './queries'

type Row = [label: string, value: ReactNode]

/**
 * The Details tab of a contact: what its form holds besides what the card
 * shows, in the form's sections.
 */
export function ContactDetails({ administrationId, contact: c }: { administrationId: string; contact: Contact }) {
  const { t, i18n } = useTranslation()
  const workflows = useWorkflows(administrationId)
  const f = (key: string) => t(`contacts.form.${key}`)
  const yes = t('contacts.view.yes')
  const workflow = (id: string | number | null | undefined) => (id ? workflows.data?.find((w) => String(w.id) === String(id))?.name ?? '' : f('noWorkflow'))
  const sepa = c.sepa_active

  return (
    <div className="contact-details">
      <Section icon="card" title={f('contactTitle')} rows={[
        [f('companyName'), typeof c.company_name === 'string' && c.company_name],
        [f('firstname'), c.firstname],
        [f('lastname'), c.lastname],
        [f('customerId'), c.customer_id],
        [f('phone'), c.phone],
        [f('taxNumber'), c.tax_number],
        [f('chamberOfCommerce'), c.chamber_of_commerce],
      ]} />
      <Section icon="send" title={f('sendingTitle')} rows={[
        [f('deliveryMethod'), c.delivery_method && t(`contacts.form.delivery.${c.delivery_method}`)],
        [f('ubl'), c.email_ubl && f('ublText')],
        [f('siIdentifier'), c.si_identifier && `${c.si_identifier_type ?? ''} ${c.si_identifier}`.trim()],
        [f('invoiceAttention'), c.send_invoices_to_attention],
        [f('invoiceEmail'), c.send_invoices_to_email],
        [f('estimateAttention'), c.send_estimates_to_attention],
        [f('estimateEmail'), c.send_estimates_to_email],
      ]} />
      <Section icon="wallet" title={f('paymentTitle')} rows={[
        [f('iban'), formatIBAN(c.sepa_iban ?? null)],
        [f('accountName'), c.sepa_iban_account_name],
        [f('bic'), c.sepa_bic],
        [f('bankAccount'), c.bank_account],
        [f('directDebit'), c.direct_debit && yes],
        [f('mandate'), sepa && [c.sepa_mandate_id, c.sepa_mandate_date].filter(Boolean).join(' · ')],
        [f('sequenceType'), sepa && c.sepa_sequence_type && t(`contacts.sequence.${c.sepa_sequence_type}`)],
        [f('trusted'), c.is_trusted && yes],
        [t('contacts.view.maxTransfer'), c.max_transfer_amount && formatAmount(c.max_transfer_amount, 'EUR', i18n.language)],
      ]} />
      <Section icon="flow" title={f('workflowTitle')} rows={[
        [f('invoiceWorkflow'), workflow(c.invoice_workflow_id)],
        [f('estimateWorkflow'), workflow(c.estimate_workflow_id)],
      ]} />
      <Section icon="fields" title={f('customFieldsTitle')} rows={(c.custom_fields ?? []).map((field): Row => [field.name ?? '', field.value])} />
    </div>
  )
}

/** A section of details, left out when it says nothing. */
function Section({ icon, title, rows }: { icon: ContactIconKind; title: string; rows: Row[] }) {
  const heading = useId()
  const shown = rows.filter(([, value]) => value !== undefined && value !== null && value !== false && value !== '')
  if (shown.length === 0) {
    return null
  }
  return (
    <section className="contact-details__section" aria-labelledby={heading}>
      <header className="contact-details__header">
        <span className="contact-details__icon"><ContactIcon kind={icon} /></span>
        <h3 id={heading} className="contact-details__title">{title}</h3>
      </header>
      <dl className="contact-details__rows">
        {shown.map(([label, value]) => (
          <div key={label} className="contact-details__row">
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
