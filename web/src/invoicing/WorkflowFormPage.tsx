import { type FormEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { type Administration, ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormNotice, FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { useOnboardingOptions } from '../onboarding/queries'
import { SearchableSelect } from '../onboarding/SearchableSelect'
import { TextileEditor } from '../forms/TextileEditor'
import { useDeleteWorkflow, useSaveWorkflow, useWorkflow, type Workflow } from './queries'
import { WorkflowPage } from './WorkflowPage'

type Kind = 'InvoiceWorkflow' | 'EstimateWorkflow'
type Reminder = { reminder_delay: number; reminder_text: string; reminder_auto_send: boolean }
type Form = {
  name: string
  language: string
  currency: string
  active: boolean
  default: boolean
  prices_are_incl_tax: boolean
  show_qr_code: boolean
  term: number
  reminders: Reminder[]
  email_subject: string
  email_body: string
  /** The payment conditions new invoices start with; empty takes the default of the language. */
  payment_conditions: string | null
}

const newKinds: Record<string, Kind> = { invoice_workflow: 'InvoiceWorkflow', estimate_workflow: 'EstimateWorkflow' }

function toForm(w: Workflow | null, a: Administration, kind: Kind): Form {
  const steps = w?.steps ?? []
  return {
    name: w?.name ?? '',
    language: w?.language ?? a.language,
    currency: w?.currency ?? a.currency,
    active: w?.active ?? true,
    default: w?.default ?? false,
    prices_are_incl_tax: w?.prices_are_incl_tax ?? false,
    show_qr_code: w?.show_qr_code ?? kind === 'InvoiceWorkflow',
    term: steps[0]?.due_interval ?? (kind === 'InvoiceWorkflow' ? 14 : 30),
    email_subject: w?.invoice_email_subject ?? '',
    email_body: w?.invoice_email_body ?? '',
    payment_conditions: w?.payment_conditions ?? null,
    reminders: steps.slice(1).map((s) => ({ reminder_delay: s.reminder_delay ?? 7, reminder_text: s.reminder_text ?? '', reminder_auto_send: s.reminder_auto_send ?? false })),
  }
}

/** Adds a workflow (/workflows/new/:kind) or edits one (/workflows/:id). */
export function WorkflowFormPage() {
  const { administrationId = '', workflowId, kind = '' } = useParams()
  const administration = useAdministration(administrationId)
  const existing = useWorkflow(administrationId, workflowId)
  if ((!workflowId && !newKinds[kind]) || (existing.error instanceof ApiError && existing.error.status < 500)) {
    return <NotFoundPage />
  }
  if (!administration.data || (workflowId && !existing.data)) {
    return <Loading />
  }
  const w = existing.data ?? null
  if (w) {
    return <WorkflowPage key={w.id} administration={administration.data} workflow={w} />
  }
  return <WorkflowForm key={kind} administration={administration.data} workflow={null} kind={newKinds[kind]!} />
}

function WorkflowForm({ administration, workflow, kind }: { administration: Administration; workflow: Workflow | null; kind: Kind }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const options = useOnboardingOptions()
  const [form, setForm] = useState(() => toForm(workflow, administration, kind))
  const save = useSaveWorkflow(administration.id, workflow?.id)
  const remove = useDeleteWorkflow(administration.id)
  const back = `/${administration.id}/settings/invoices_and_estimates#workflows`
  const invoices = kind === 'InvoiceWorkflow'
  const isDefault = workflow?.default ?? false
  const labels = {
    name: t('workflow.name'), language: t('workflow.language'), currency: t('workflow.currency'), active: t('workflow.active'), steps: invoices ? t('workflow.reminders') : t('workflow.validity'),
    invoice_email_subject: t('workflow.emailSubject'), invoice_email_body: t('workflow.emailBody'), payment_conditions: t('workflow.page.conditions'),
  }
  const errors = formErrors(save.error ?? remove.error, labels)

  const currencyOptions = useMemo(() => {
    const names = new Intl.DisplayNames([i18n.language], { type: 'currency', fallback: 'code' })
    return (options.data?.currencies ?? [form.currency]).map((value) => ({ value, label: `${value} - ${names.of(value) ?? value}` }))
  }, [options.data, form.currency, i18n.language])

  const setReminder = (i: number, change: Partial<Reminder>) =>
    setForm({ ...form, reminders: form.reminders.map((r, j) => (j === i ? { ...r, ...change } : r)) })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(
      {
        ...(workflow ? {} : { type: kind }),
        name: form.name, language: form.language as 'en', currency: form.currency,
        prices_are_incl_tax: form.prices_are_incl_tax, show_qr_code: form.show_qr_code,
        ...(isDefault ? {} : { active: form.active, default: form.default }),
        ...(invoices ? { invoice_email_subject: form.email_subject.trim(), invoice_email_body: form.email_body } : {}),
        // Left out, the server starts the workflow with Moneybird's text in its language.
        ...(invoices && form.payment_conditions !== null ? { payment_conditions: form.payment_conditions.trim() } : {}),
        steps_attributes: [{ due_interval: form.term }, ...(invoices ? form.reminders : [])],
      },
      { onSuccess: () => navigate(back) },
    )
  }

  const title = workflow ? t('workflow.edit') : invoices ? t('workflow.newInvoice') : t('workflow.newEstimate')
  const deleteAction = workflow && !isDefault && (
    <ConfirmDelete label={t('settings.delete')} confirmLabel={t('settings.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(workflow.id ?? '', { onSuccess: () => navigate(back) })} />
  )

  return (
    <Page title={title} back={{ to: back, label: t('workflow.back') }} actions={deleteAction || undefined}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        {isDefault && <FormNotice>{t('workflow.defaultLocked')}</FormNotice>}
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <SelectInput label={labels.language} width="medium" value={form.language} errors={errors.field('language')} onChange={(e) => setForm({ ...form, language: e.target.value })}>
            <option value="nl">Nederlands</option>
            <option value="nl-be">Nederlands (België)</option>
            <option value="en">English</option>
          </SelectInput>
          <SearchableSelect
            label={labels.currency}
            searchLabel={t('onboarding.findCurrency')}
            emptyLabel={t('onboarding.noOption')}
            width="medium"
            value={form.currency}
            options={currencyOptions}
            errors={errors.field('currency')}
            onChange={(currency) => setForm({ ...form, currency })}
          />
          <CheckboxInput label={t('workflow.pricesInclTax')} checked={form.prices_are_incl_tax} onChange={(e) => setForm({ ...form, prices_are_incl_tax: e.target.checked })} />
          {invoices && <CheckboxInput label={t('workflow.showQrCode')} checked={form.show_qr_code} onChange={(e) => setForm({ ...form, show_qr_code: e.target.checked })} />}
          {!isDefault && (
            <>
              <CheckboxInput label={labels.active} checked={form.active} errors={errors.field('active')} onChange={(e) => setForm({ ...form, active: e.target.checked, default: e.target.checked && form.default })} />
              <CheckboxInput label={t('workflow.default')} checked={form.default} disabled={!form.active} onChange={(e) => setForm({ ...form, default: e.target.checked })} />
            </>
          )}
        </FormSection>

        <FormSection>
          <TextInput
            label={invoices ? t('workflow.term') : t('workflow.validity')}
            type="number"
            inputMode="numeric"
            min={0}
            max={365}
            width="small"
            help={t('workflow.days')}
            value={String(form.term)}
            errors={invoices ? undefined : errors.field('steps')}
            onChange={(e) => setForm({ ...form, term: Number(e.target.value) })}
          />
        </FormSection>

        {invoices && (
          <FormSection title={t('workflow.page.conditions')}>
            <TextileEditor
              label={t('workflow.page.conditions')}
              value={form.payment_conditions ?? t('workflow.defaultConditions', { lng: form.language.startsWith('nl') ? 'nl' : 'en' })}
              maxLength={1000}
              errors={errors.field('payment_conditions')}
              onChange={(payment_conditions) => setForm({ ...form, payment_conditions })}
            />
            <p className="form-input__message form-input__message--help">{t('workflow.conditionsHelp')}</p>
          </FormSection>
        )}

        {invoices && (
          <FormSection title={t('workflow.email')}>
            <TextInput
              label={labels.invoice_email_subject}
              maxLength={255}
              autoComplete="off"
              help={t('workflow.emailSubjectHelp')}
              value={form.email_subject}
              errors={errors.field('invoice_email_subject')}
              onChange={(e) => setForm({ ...form, email_subject: e.target.value })}
            />
            <TextArea
              label={labels.invoice_email_body}
              rows={6}
              maxLength={10000}
              help={t('workflow.emailBodyHelp')}
              value={form.email_body}
              errors={errors.field('invoice_email_body')}
              onChange={(e) => setForm({ ...form, email_body: e.target.value })}
            />
          </FormSection>
        )}

        {invoices && (
          <FormSection title={t('workflow.reminders')}>
            {errors.field('steps') && <Problems messages={errors.field('steps') ?? []} />}
            {form.reminders.length === 0 && <p className="empty">{t('workflow.noReminders')}</p>}
            {form.reminders.map((r, i) => (
              <fieldset className="reminder" key={i}>
                <legend className="reminder__title">{t('workflow.reminder', { n: i + 1 })}</legend>
                <TextInput
                  label={t('workflow.reminderDelay')}
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={365}
                  width="small"
                  value={String(r.reminder_delay)}
                  onChange={(e) => setReminder(i, { reminder_delay: Number(e.target.value) })}
                />
                <TextArea label={t('workflow.reminderText')} maxLength={5000} value={r.reminder_text} onChange={(e) => setReminder(i, { reminder_text: e.target.value })} />
                <CheckboxInput label={t('workflow.autoSend')} checked={r.reminder_auto_send} onChange={(e) => setReminder(i, { reminder_auto_send: e.target.checked })} />
                <button type="button" className="btn btn--secondary btn--small reminder__remove" onClick={() => setForm({ ...form, reminders: form.reminders.filter((_, j) => j !== i) })}>
                  {t('workflow.removeReminder', { n: i + 1 })}
                </button>
              </fieldset>
            ))}
            {form.reminders.length < 10 && (
              <button
                type="button"
                className="icon-link icon-link--button"
                onClick={() => setForm({ ...form, reminders: [...form.reminders, { reminder_delay: 7, reminder_text: '', reminder_auto_send: false }] })}
              >
                {t('workflow.addReminder')}
              </button>
            )}
          </FormSection>
        )}

        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
