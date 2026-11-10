import { type FormEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { useOnboardingOptions } from '../onboarding/queries'
import { SearchableSelect } from '../onboarding/SearchableSelect'
import { countryName } from '../settings/format'
import { useCustomFields } from '../settings/queries'
import { type Contact, type ContactChanges, contactName, useContact, useSaveContact } from './queries'

export const deliveryMethods = ['Email', 'Post', 'Manual', 'Peppol', 'Simplerinvoicing'] as const
const eInvoicing = ['Peppol', 'Simplerinvoicing']
const sequenceTypes = ['RCUR', 'FRST', 'OOFF', 'FNAL'] as const

const textFields = [
  'company_name', 'firstname', 'lastname', 'customer_id', 'address1', 'address2', 'zipcode', 'city', 'country', 'phone',
  'send_invoices_to_email', 'send_invoices_to_attention', 'send_estimates_to_email', 'send_estimates_to_attention',
  'tax_number', 'chamber_of_commerce', 'bank_account', 'si_identifier_type', 'si_identifier',
  'sepa_iban', 'sepa_iban_account_name', 'sepa_bic', 'sepa_mandate_id', 'sepa_mandate_date',
] as const
type TextField = (typeof textFields)[number]

/** Adds a contact (/contacts/new) or edits one (/contacts/:id/edit). */
export function ContactFormPage() {
  const { administrationId = '', contactId } = useParams()
  const contact = useContact(administrationId, contactId)
  const administration = useAdministration(administrationId)
  if (contactId && contact.error instanceof ApiError && contact.error.status < 500) {
    return <NotFoundPage />
  }
  if ((contactId && !contact.data) || !administration.data) {
    return <Loading />
  }
  return <ContactForm key={contact.data?.id ?? 'new'} administrationId={administrationId} contact={contact.data ?? null} country={administration.data.country} />
}

function ContactForm({ administrationId, contact, country }: { administrationId: string; contact: Contact | null; country: string }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveContact(administrationId, contact?.id)
  const workflows = useWorkflows(administrationId)
  const fields = useCustomFields(administrationId)
  const options = useOnboardingOptions()
  const [text, setText] = useState<Record<TextField, string>>(() => {
    const initial = Object.fromEntries(textFields.map((f) => [f, String(contact?.[f] ?? '')])) as Record<TextField, string>
    initial.country = contact?.country ?? country
    return initial
  })
  const [delivery, setDelivery] = useState<string>(contact?.delivery_method ?? 'Email')
  const [emailUBL, setEmailUBL] = useState(contact?.email_ubl ?? false)
  const [mandate, setMandate] = useState(contact?.sepa_active ?? false)
  const [sequence, setSequence] = useState<string>(contact?.sepa_sequence_type ?? 'RCUR')
  const [invoiceWorkflow, setInvoiceWorkflow] = useState(contact?.invoice_workflow_id ?? '')
  const [estimateWorkflow, setEstimateWorkflow] = useState(contact?.estimate_workflow_id ?? '')
  const contactFields = (fields.data ?? []).filter((f) => f.source === 'contact')
  const [custom, setCustom] = useState<Record<string, string>>(() => Object.fromEntries((contact?.custom_fields ?? []).map((f) => [f.id ?? '', f.value ?? ''])))

  const countryOptions = useMemo(
    () => (options.data?.countries ?? [text.country]).map((value) => ({ value, label: countryName(i18n.language, value) })).sort((a, b) => a.label.localeCompare(b.label, i18n.language)),
    [options.data, text.country, i18n.language],
  )
  const labels: Record<string, string> = {
    company_name: t('contacts.companyName'), firstname: t('contacts.firstname'), lastname: t('contacts.lastname'), customer_id: t('contacts.customerId'),
    address1: t('contacts.address'), address2: t('contacts.address2'), zipcode: t('contacts.zipcode'), city: t('contacts.city'), country: t('contacts.country'),
    phone: t('contacts.phone'), send_invoices_to_email: t('contacts.invoiceEmail'), send_invoices_to_attention: t('contacts.attention'),
    send_estimates_to_email: t('contacts.estimateEmail'), send_estimates_to_attention: t('contacts.estimateAttention'), tax_number: t('contacts.taxNumber'),
    chamber_of_commerce: t('contacts.chamberOfCommerce'), bank_account: t('contacts.bankAccount'), si_identifier_type: t('contacts.siIdentifierType'),
    si_identifier: t('contacts.siIdentifier'), sepa_iban: t('contacts.iban'), sepa_iban_account_name: t('contacts.accountName'), sepa_bic: t('contacts.bic'),
    sepa_mandate_id: t('contacts.mandateId'), sepa_mandate_date: t('contacts.mandateDate'), sepa_sequence_type: t('contacts.sequenceType'),
    delivery_method: t('contacts.deliveryMethod'), invoice_workflow_id: t('contacts.invoiceWorkflow'), estimate_workflow_id: t('contacts.estimateWorkflow'),
  }
  const errors = formErrors(save.error, labels)
  const input = (field: TextField, extra: Partial<React.ComponentProps<typeof TextInput>> = {}) => (
    <TextInput label={labels[field] ?? field} maxLength={255} autoComplete="off" value={text[field]} errors={errors.field(field)} onChange={(e) => setText({ ...text, [field]: e.target.value })} {...extra} />
  )

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body: ContactChanges = {
      ...Object.fromEntries(textFields.filter((f) => !f.startsWith('sepa_') || mandate).map((f) => [f, text[f].trim()])),
      delivery_method: delivery as NonNullable<ContactChanges['delivery_method']>,
      email_ubl: emailUBL,
      sepa_active: mandate,
      direct_debit: mandate,
      sepa_sequence_type: sequence as NonNullable<ContactChanges['sepa_sequence_type']>,
      invoice_workflow_id: invoiceWorkflow,
      estimate_workflow_id: estimateWorkflow,
      ...(contactFields.length > 0 ? { custom_fields_attributes: contactFields.map((f) => ({ id: f.id, value: custom[f.id] ?? '' })) } : {}),
    }
    if (!contact && !body.customer_id) {
      delete body.customer_id
    }
    save.mutate(body, { onSuccess: (saved) => navigate(`/${administrationId}/contacts/${saved.id}`) })
  }

  const back = contact ? `/${administrationId}/contacts/${contact.id}` : `/${administrationId}/contacts`
  const flows = (type: string) => (workflows.data ?? []).filter((w) => w.type === type && (w.active || w.id === invoiceWorkflow || w.id === estimateWorkflow))

  return (
    <Page title={contact ? t('contacts.editTitle', { name: contactName(contact) }) : t('contacts.newTitle')} back={{ to: back, label: t('contacts.back') }}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          {input('company_name')}
          {input('firstname')}
          {input('lastname')}
          {input('customer_id', { width: 'medium', maxLength: 50, placeholder: contact ? undefined : t('contacts.nextNumber') })}
        </FormSection>
        <FormSection title={t('contacts.addressTitle')}>
          {input('address1')}
          {input('address2')}
          {input('zipcode', { width: 'small' })}
          {input('city')}
          <SearchableSelect label={labels.country ?? ''} searchLabel={t('onboarding.findCountry')} emptyLabel={t('onboarding.noOption')} value={text.country} options={countryOptions} errors={errors.field('country')} onChange={(value) => setText({ ...text, country: value })} />
          {input('phone', { type: 'tel', width: 'medium' })}
        </FormSection>
        <FormSection title={t('contacts.invoicingTitle')}>
          <SelectInput label={labels.delivery_method ?? ''} width="medium" value={delivery} errors={errors.field('delivery_method')} onChange={(e) => setDelivery(e.target.value)}>
            {deliveryMethods.map((m) => <option key={m} value={m}>{t(`contacts.delivery.${m}`)}</option>)}
          </SelectInput>
          {input('send_invoices_to_email', { type: 'email', help: t('contacts.emailHelp') })}
          {input('send_invoices_to_attention')}
          {delivery === 'Email' && <CheckboxInput label={t('contacts.emailUBL')} checked={emailUBL} onChange={(e) => setEmailUBL(e.target.checked)} />}
          {eInvoicing.includes(delivery) && (
            <>
              {input('si_identifier_type', { width: 'small', maxLength: 20, help: t('contacts.siTypeHelp') })}
              {input('si_identifier')}
            </>
          )}
          {input('send_estimates_to_email', { type: 'email' })}
          {input('send_estimates_to_attention')}
          <SelectInput label={labels.invoice_workflow_id ?? ''} width="medium" value={invoiceWorkflow} errors={errors.field('invoice_workflow_id')} onChange={(e) => setInvoiceWorkflow(e.target.value)}>
            <option value="">{t('contacts.defaultWorkflow')}</option>
            {flows('InvoiceWorkflow').map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
          </SelectInput>
          <SelectInput label={labels.estimate_workflow_id ?? ''} width="medium" value={estimateWorkflow} errors={errors.field('estimate_workflow_id')} onChange={(e) => setEstimateWorkflow(e.target.value)}>
            <option value="">{t('contacts.defaultWorkflow')}</option>
            {flows('EstimateWorkflow').map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
          </SelectInput>
        </FormSection>
        <FormSection title={t('contacts.companyTitle')}>
          {input('tax_number', { width: 'medium' })}
          {input('chamber_of_commerce', { width: 'medium' })}
          {input('bank_account', { width: 'medium' })}
        </FormSection>
        <FormSection title={t('contacts.mandateTitle')}>
          <CheckboxInput label={t('contacts.mandateOn')} checked={mandate} errors={errors.field('sepa_active')} onChange={(e) => setMandate(e.target.checked)} />
          {mandate && (
            <>
              {input('sepa_iban', { width: 'medium' })}
              {input('sepa_iban_account_name')}
              {input('sepa_bic', { width: 'small' })}
              {input('sepa_mandate_id', { width: 'medium' })}
              {input('sepa_mandate_date', { type: 'date', width: 'medium' })}
              <SelectInput label={labels.sepa_sequence_type ?? ''} width="medium" value={sequence} errors={errors.field('sepa_sequence_type')} onChange={(e) => setSequence(e.target.value)}>
                {sequenceTypes.map((s) => <option key={s} value={s}>{t(`contacts.sequence.${s}`)}</option>)}
              </SelectInput>
            </>
          )}
        </FormSection>
        {contactFields.length > 0 && (
          <FormSection title={t('contacts.customFields')}>
            {contactFields.map((f, i) => (
              <TextInput key={f.id} label={f.name} maxLength={255} value={custom[f.id] ?? ''} errors={errors.field(`custom_fields_attributes.${i}`)} onChange={(e) => setCustom({ ...custom, [f.id]: e.target.value })} />
            ))}
          </FormSection>
        )}
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('contacts.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
