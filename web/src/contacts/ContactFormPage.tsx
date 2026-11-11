import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formErrors, SelectInput, TextInput } from '../forms/Form'
import { useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { useOnboardingOptions } from '../onboarding/queries'
import { SearchableSelect } from '../onboarding/SearchableSelect'
import { countryName } from '../settings/format'
import { useCustomFields } from '../settings/queries'
import { CompanyNameField } from './CompanyNameField'
import { ContactIcon } from './ContactIcon'
import { CheckRow, Counter, Section, SegmentedRow, Tip } from './ContactFormParts'
import { type Contact, type ContactChanges, useArchiveContact, useContact, useNextCustomerId, useSaveContact } from './queries'
import './contacts.css'

export const deliveryMethods = ['Email', 'Post', 'Manual', 'Peppol', 'Simplerinvoicing'] as const
type DeliveryMethod = (typeof deliveryMethods)[number]
/** The delivery methods Moneybird's form offers; the others stay when a contact already has one. */
const offered: DeliveryMethod[] = ['Email', 'Manual', 'Peppol']
const eInvoicing: string[] = ['Peppol', 'Simplerinvoicing']
const sequenceTypes = ['RCUR', 'FRST', 'OOFF', 'FNAL'] as const
const accountNameMax = 70

const textFields = [
  'company_name', 'firstname', 'lastname', 'customer_id', 'address1', 'address2', 'zipcode', 'city', 'country', 'phone',
  'send_invoices_to_email', 'send_invoices_to_attention', 'send_estimates_to_email', 'send_estimates_to_attention',
  'tax_number', 'chamber_of_commerce', 'bank_account', 'si_identifier_type', 'si_identifier',
  'sepa_iban', 'sepa_iban_account_name', 'sepa_bic', 'sepa_mandate_id', 'sepa_mandate_date',
] as const
type TextField = (typeof textFields)[number]
/** Fields of the mandate, sent only while the contact has one so a contact without keeps what it held. */
const mandateFields: TextField[] = ['sepa_mandate_id', 'sepa_mandate_date']

type Kind = 'company' | 'private_individual'

/** Adds a contact (/contacts/new) or edits one (/contacts/:id/edit), on Moneybird's form. */
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
  const archive = useArchiveContact(administrationId, contact?.id ?? '')
  const workflows = useWorkflows(administrationId)
  const fields = useCustomFields(administrationId)
  const options = useOnboardingOptions()
  const [text, setText] = useState<Record<TextField, string>>(() => {
    const initial = Object.fromEntries(textFields.map((f) => [f, String(contact?.[f] ?? '')])) as Record<TextField, string>
    initial.country = contact?.country ?? country
    return initial
  })
  // A new contact shows the client number it will get, as Moneybird does.
  const nextCustomerId = useNextCustomerId(administrationId, !contact)
  const [prefilled, setPrefilled] = useState('')
  useEffect(() => {
    const next = nextCustomerId.data
    if (next) {
      // Only while the field still shows what was filled in, never over what the user typed.
      setText((current) => (current.customer_id === prefilled ? { ...current, customer_id: next } : current))
      setPrefilled(next)
    }
  }, [nextCustomerId.data]) // eslint-disable-line react-hooks/exhaustive-deps
  const [kind, setKind] = useState<Kind>(contact && !contact.company_name && (contact.firstname || contact.lastname) ? 'private_individual' : 'company')
  const [delivery, setDelivery] = useState<DeliveryMethod>((contact?.delivery_method as DeliveryMethod | undefined) ?? 'Email')
  const [emailUBL, setEmailUBL] = useState(contact?.email_ubl ?? false)
  const [sameForQuotes, setSameForQuotes] = useState(() => {
    const quotes = [contact?.send_estimates_to_attention ?? '', contact?.send_estimates_to_email ?? '']
    return quotes.every((v) => !v) || (quotes[0] === (contact?.send_invoices_to_attention ?? '') && quotes[1] === (contact?.send_invoices_to_email ?? ''))
  })
  const [secondLine, setSecondLine] = useState(Boolean(contact?.address2))
  const [directDebit, setDirectDebit] = useState(contact?.direct_debit ?? false)
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
  const f = (key: string) => t(`contacts.form.${key}`)
  const labels: Record<string, string> = {
    company_name: f('companyName'), chamber_of_commerce: f('chamberOfCommerce'), tax_number: f('taxNumber'),
    firstname: kind === 'company' ? f('personFirstname') : f('firstname'), lastname: kind === 'company' ? f('personLastname') : f('lastname'),
    phone: f('phone'), customer_id: f('customerId'), address1: f('address'), address2: f('address2'), zipcode: f('zipcode'), city: f('city'),
    country: f('country'), delivery_method: f('deliveryMethod'), si_identifier_type: f('siIdentifierType'), si_identifier: f('siIdentifier'),
    send_invoices_to_attention: f('invoiceAttention'), send_invoices_to_email: f('invoiceEmail'),
    send_estimates_to_attention: f('estimateAttention'), send_estimates_to_email: f('estimateEmail'),
    sepa_iban: f('iban'), sepa_bic: f('bic'), sepa_iban_account_name: f('accountName'), bank_account: f('bankAccount'),
    direct_debit: f('directDebit'), sepa_active: f('mandate'), sepa_mandate_id: f('mandateId'), sepa_mandate_date: f('mandateDate'),
    sepa_sequence_type: f('sequenceType'), invoice_workflow_id: f('invoiceWorkflow'), estimate_workflow_id: f('estimateWorkflow'),
  }
  const errors = formErrors(save.error, labels)
  const change = (field: TextField, value: string) => setText((current) => ({ ...current, [field]: value }))
  const input = (field: TextField, extra: Partial<React.ComponentProps<typeof TextInput>> = {}) => (
    <TextInput label={labels[field] ?? field} maxLength={255} autoComplete="off" value={text[field]} errors={errors.field(field)} onChange={(e) => change(field, e.target.value)} {...extra} />
  )
  const ibanAbroad = /^[A-Z]{2}/i.test(text.sepa_iban.trim()) && !/^NL/i.test(text.sepa_iban.trim())

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const trimmed = Object.fromEntries(textFields.filter((k) => mandate || !mandateFields.includes(k)).map((k) => [k, text[k].trim()])) as Partial<Record<TextField, string>>
    const body: ContactChanges = {
      ...trimmed,
      ...(kind === 'private_individual' ? { company_name: '' } : {}),
      ...(sameForQuotes ? { send_estimates_to_attention: trimmed.send_invoices_to_attention, send_estimates_to_email: trimmed.send_invoices_to_email } : {}),
      delivery_method: delivery,
      email_ubl: emailUBL,
      direct_debit: directDebit,
      sepa_active: mandate,
      ...(mandate ? { sepa_sequence_type: sequence as NonNullable<ContactChanges['sepa_sequence_type']> } : {}),
      invoice_workflow_id: invoiceWorkflow,
      estimate_workflow_id: estimateWorkflow,
      ...(contactFields.length > 0 ? { custom_fields_attributes: contactFields.map((c) => ({ id: c.id, value: custom[c.id] ?? '' })) } : {}),
    }
    // Left as filled in, the number is the server's to give: another contact may have taken it meanwhile.
    if (!contact && (!body.customer_id || body.customer_id === prefilled)) {
      delete body.customer_id
    }
    save.mutate(body, { onSuccess: (saved) => navigate(`/${administrationId}/contacts/${saved.id}`) })
  }

  const back = contact ? `/${administrationId}/contacts/${contact.id}` : `/${administrationId}/contacts`
  const newField = `/${administrationId}/custom_fields/new?source=contact`
  const flows = (type: string) => (workflows.data ?? []).filter((w) => w.type === type && (w.active || w.id === invoiceWorkflow || w.id === estimateWorkflow))
  const title = contact ? f('editTitle') : f('newTitle')
  const methods = offered.includes(delivery) ? offered : [...offered, delivery]

  return (
    <Page
      title={title}
      actions={
        <MoreMenu items={[
          { label: f('addCustomField'), to: newField },
          ...(contact && !contact.archived ? [{ label: t('contacts.archive'), onSelect: () => archive.mutate(undefined, { onSuccess: () => navigate(back) }) }] : []),
        ]} />
      }
    >
      <form className="contact-form" aria-label={title} onSubmit={submit} noValidate>
        <Problems messages={[...errors.general, ...formErrors(archive.error, {}).general]} />
        <div className="contact-form__sheet">
          <Section icon="card" title={f('contactTitle')}>
            <SegmentedRow<Kind> label={f('type')} value={kind} onChange={setKind}
              options={[{ value: 'company', label: f('company') }, { value: 'private_individual', label: f('private') }]} />
            {kind === 'company' ? (
              <>
                <CompanyNameField
                  label={labels.company_name ?? ''}
                  value={text.company_name}
                  errors={errors.field('company_name')}
                  registers={contact ? [] : (options.data?.company_search_countries ?? [])}
                  onChange={(value) => change('company_name', value)}
                  onChoose={(company, register) => setText((current) => ({
                    ...current, company_name: company.name, chamber_of_commerce: company.chamber_of_commerce || current.chamber_of_commerce,
                    address1: company.address1 || current.address1, zipcode: company.zipcode || current.zipcode, city: company.city || current.city, country: register,
                  }))}
                />
                {input('chamber_of_commerce', { width: 'medium' })}
                {input('tax_number', { width: 'medium', labelExtra: <Tip text={f('taxNumberTip')} /> })}
              </>
            ) : (
              <>
                {input('firstname', { width: 'medium' })}
                {input('lastname', { width: 'medium' })}
              </>
            )}
            {input('phone', { type: 'tel', width: 'medium' })}
            {input('customer_id', { width: 'medium', maxLength: 50 })}
            {kind === 'company' && (
              <fieldset className="contact-form__person">
                <legend className="contact-form__subtitle">{f('contactPerson')}</legend>
                {input('firstname', { width: 'small' })}
                {input('lastname', { width: 'small' })}
              </fieldset>
            )}
          </Section>

          <Section icon="pin" title={f('addressTitle')}>
            {input('address1', {
              action: secondLine ? undefined : (
                <button type="button" className="icon-btn contact-form__inline-btn" aria-label={f('addAddressLine')} title={f('addAddressLine')} onClick={() => setSecondLine(true)}>
                  <ContactIcon kind="plus" />
                </button>
              ),
            })}
            {secondLine && input('address2', { autoFocus: !contact?.address2 })}
            {input('zipcode', { width: 'small' })}
            {input('city')}
            <SearchableSelect label={labels.country ?? ''} searchLabel={t('onboarding.findCountry')} emptyLabel={t('onboarding.noOption')} value={text.country} options={countryOptions} errors={errors.field('country')} onChange={(value) => change('country', value)} />
          </Section>

          <Section icon="send" title={f('sendingTitle')}>
            <SegmentedRow<DeliveryMethod> label={labels.delivery_method ?? ''} value={delivery} errors={errors.field('delivery_method')} onChange={setDelivery}
              options={methods.map((m) => ({ value: m, label: f(`delivery.${m}`) }))} />
            {delivery === 'Email' && <CheckRow label={f('ubl')} text={f('ublText')} checked={emailUBL} errors={errors.field('email_ubl')} onChange={(e) => setEmailUBL(e.target.checked)} />}
            {eInvoicing.includes(delivery) && (
              <>
                {input('si_identifier_type', { width: 'small', maxLength: 20, help: f('siTypeHelp') })}
                {input('si_identifier', { width: 'medium' })}
              </>
            )}
            {input('send_invoices_to_attention', { width: 'medium' })}
            {input('send_invoices_to_email', { type: 'email', help: f('emailHelp') })}
            <CheckRow label={f('quotes')} text={f('sameForQuotes')} checked={sameForQuotes} onChange={(e) => setSameForQuotes(e.target.checked)} />
            {!sameForQuotes && (
              <>
                {input('send_estimates_to_attention', { width: 'medium' })}
                {input('send_estimates_to_email', { type: 'email' })}
              </>
            )}
          </Section>

          <Section icon="wallet" title={f('paymentTitle')}>
            {input('sepa_iban', { placeholder: f('ibanPlaceholder'), maxLength: 64 })}
            {(ibanAbroad || Boolean(contact?.sepa_bic)) && input('sepa_bic', { width: 'small', maxLength: 16, placeholder: f('bicPlaceholder') })}
            {input('sepa_iban_account_name', { maxLength: accountNameMax, action: <Counter length={text.sepa_iban_account_name.length} max={accountNameMax} /> })}
            {input('bank_account', { width: 'medium' })}
            <CheckRow label={labels.direct_debit ?? ''} tip={f('directDebitTip')} text={f('directDebitText')} checked={directDebit} errors={errors.field('direct_debit')} onChange={(e) => setDirectDebit(e.target.checked)} />
            <CheckRow label={labels.sepa_active ?? ''} text={f('mandateText')} checked={mandate} errors={errors.field('sepa_active')} onChange={(e) => setMandate(e.target.checked)} />
            {mandate && (
              <>
                {input('sepa_mandate_id', { width: 'medium' })}
                {input('sepa_mandate_date', { type: 'date', width: 'medium' })}
                <SelectInput label={labels.sepa_sequence_type ?? ''} width="medium" value={sequence} errors={errors.field('sepa_sequence_type')} onChange={(e) => setSequence(e.target.value)}>
                  {sequenceTypes.map((s) => <option key={s} value={s}>{t(`contacts.sequence.${s}`)}</option>)}
                </SelectInput>
              </>
            )}
            <CheckRow label={f('trusted')} text={f('trustedText')} muted disabled checked={contact?.is_trusted ?? false} readOnly />
          </Section>

          <Section icon="flow" title={f('workflowTitle')}>
            <SelectInput label={labels.invoice_workflow_id ?? ''} width="medium" value={invoiceWorkflow} errors={errors.field('invoice_workflow_id')} onChange={(e) => setInvoiceWorkflow(e.target.value)}>
              <option value="">{f('noWorkflow')}</option>
              {flows('InvoiceWorkflow').map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
            </SelectInput>
            <SelectInput label={labels.estimate_workflow_id ?? ''} width="medium" value={estimateWorkflow} errors={errors.field('estimate_workflow_id')} onChange={(e) => setEstimateWorkflow(e.target.value)}>
              <option value="">{f('noWorkflow')}</option>
              {flows('EstimateWorkflow').map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
            </SelectInput>
          </Section>

          <Section
            icon="fields"
            title={f('customFieldsTitle')}
            action={
              <Link className="btn btn--secondary btn--with-icon" to={newField}>
                <ContactIcon kind="plus" />
                <span>{f('addCustomField')}</span>
              </Link>
            }
          >
            {contactFields.length === 0 ? (
              <p className="contact-form__text">{f('customFieldsText')}</p>
            ) : (
              contactFields.map((c, i) => (
                <TextInput key={c.id} label={c.name} maxLength={255} value={custom[c.id] ?? ''} errors={errors.field(`custom_fields_attributes.${i}`)} onChange={(e) => setCustom({ ...custom, [c.id]: e.target.value })} />
              ))
            )}
          </Section>
        </div>
        <div className="contact-form__bar">
          <button className="btn btn--primary" disabled={save.isPending}>{f('save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
