import { type FormEvent, type ReactNode, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import type { Administration } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { RadioRow } from '../forms/RadioRow'
import { TextileEditor, TextileText } from '../forms/TextileEditor'
import { conditionTagValues } from '../invoices/conditions'
import { addDays, today } from '../invoices/paper'
import { ListIcon } from '../layout/icons'
import { Page } from '../layout/Page'
import { useOnboardingOptions } from '../onboarding/queries'
import { SearchableSelect } from '../onboarding/SearchableSelect'
import { useIdentities } from '../settings/queries'
import { expandEmail } from './ChannelPreviews'
import { useDeleteWorkflow, useSaveWorkflow, type Workflow } from './queries'
import './workflow.css'

type Step = { due_interval: number; reminder_delay: number; reminder_text: string; reminder_auto_send: boolean }
type Editing = 'general' | 'payment' | 'send' | { reminder: number } | null
type Changes = Parameters<ReturnType<typeof useSaveWorkflow>['mutate']>[0]

/** The workflow's steps in order, as the API takes them back. */
function stepsOf(w: Workflow): Step[] {
  return [...(w.steps ?? [])]
    .sort((a, b) => (a.step_order ?? 0) - (b.step_order ?? 0))
    .map((s) => ({ due_interval: s.due_interval ?? 0, reminder_delay: s.reminder_delay ?? 0, reminder_text: s.reminder_text ?? '', reminder_auto_send: s.reminder_auto_send ?? false }))
}

/**
 * A workflow's page, after Moneybird's: its general and payment settings on
 * the left, and on the right its steps, from sending the invoice to the last
 * reminder, each shown in a card that opens its own editor. What Moneybird
 * has that MoneyHarpy cannot do yet is shown as not available.
 */
export function WorkflowPage({ administration, workflow: w }: { administration: Administration; workflow: Workflow }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const returnTo = params.get('return_to')
  const back = `/${administration.id}/settings/invoices_and_estimates#workflows`
  const save = useSaveWorkflow(administration.id, w.id)
  const remove = useDeleteWorkflow(administration.id)
  const [editing, setEditing] = useState<Editing>(null)
  const [tab, setTab] = useState<'sending' | 'payment'>('sending')
  const invoices = w.type === 'InvoiceWorkflow'
  const steps = stepsOf(w)
  const term = steps[0]?.due_interval ?? 0
  const reminders = steps.slice(1)
  const labels = {
    name: t('workflow.name'), language: t('workflow.language'), currency: t('workflow.currency'), active: t('workflow.active'), steps: t('workflow.reminders'),
    payment_conditions: t('workflow.page.conditions'), invoice_email_subject: t('workflow.page.emailSubject'), invoice_email_body: t('workflow.page.invoiceEmail'),
  }
  const errors = formErrors(save.error ?? remove.error, labels)

  const open = (e: Editing) => {
    save.reset()
    setEditing(e)
  }
  /** Saves the workflow with the changes, sending its steps whole as the API replaces them. */
  const patch = (over: Partial<Changes> & { steps?: Step[] }) => {
    const { steps: changedSteps, ...rest } = over
    save.mutate(
      {
        name: w.name, language: w.language as 'en', currency: w.currency, prices_are_incl_tax: w.prices_are_incl_tax, show_qr_code: w.show_qr_code,
        ...rest,
        steps_attributes: (changedSteps ?? steps).map((s, i) => (i === 0 ? { due_interval: s.due_interval } : s)),
      },
      { onSuccess: () => setEditing(null) },
    )
  }

  const identities = useIdentities(administration.id)
  const sender = identities.data?.find((i) => i.default) ?? identities.data?.[0]
  const language = w.language ?? administration.language
  const dt = i18n.getFixedT(language)
  // The texts are shown as an invoice prints them, with example amounts and a draft's reference.
  const invoiceDate = today()
  const dueDate = addDays(invoiceDate, term)
  const sampleTotal = 121000
  const values = conditionTagValues({
    language, currency: w.currency ?? administration.currency, total: sampleTotal, unpaid: sampleTotal, invoiceDate, dueDate, invoiceId: '2026-0001',
    contact: { firstname: dt('workflow.page.sample.firstname'), lastname: dt('workflow.page.sample.lastname') }, sender,
  })
  const emailValues = {
    invoice_id: '2026-0001', invoice_date: values['document.invoice_date'] ?? '', due_date: values['document.due_date'] ?? '', total: values['document.total_price'] ?? '',
    contact_name: values['recipient.contact_name'] ?? '', company_name: sender?.company_name ?? '', payment_reference: values['document.payment_reference'] ?? '', reference: '',
  }
  const email = (text: string) => expandEmail(text, emailValues, values)

  const ordinal = new Intl.PluralRules(i18n.language, { type: 'ordinal' })
  const reminderName = (i: number) => t('workflow.page.reminder', { n: i + 1, suffix: t(`workflow.page.ordinal.${ordinal.select(i + 1)}`, { defaultValue: t('workflow.page.ordinal.other') }) })
  // Day 1 the invoice goes out; each reminder follows the step before it.
  const days: number[] = []
  reminders.reduce((day, r) => {
    days.push(day + r.reminder_delay)
    return day + r.reminder_delay
  }, 1 + term)
  const intros = t('workflow.page.reminderIntro', { returnObjects: true }) as string[]
  const deleteAction = w.default ? (
    <button type="button" className="btn btn--secondary" disabled title={t('invoicing.defaultCannotBeDeleted')}>{t('invoicing.defaultCannotBeDeleted')}</button>
  ) : (
    <ConfirmDelete label={t('workflow.page.delete')} confirmLabel={t('workflow.page.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(w.id ?? '', { onSuccess: () => navigate(back) })} />
  )

  return (
    <Page
      title={t(`workflow.page.title.${invoices ? 'InvoiceWorkflow' : 'EstimateWorkflow'}`, { name: w.name })}
      back={returnTo?.startsWith('/') ? { to: returnTo, label: t('workflow.page.toReturn') } : { to: back, label: t('workflow.back') }}
      actions={deleteAction}
      wide
    >
      <Problems messages={editing ? [] : errors.general} />
      <div className="workflow-page">
        <div className="workflow-page__side">
          <Card title={t('workflow.page.general')} editLabel={t('workflow.page.editGeneral')} editing={editing === 'general'} onEdit={() => open('general')}>
            {editing === 'general' ? (
              <GeneralForm workflow={w} administration={administration} errors={errors} pending={save.isPending} onCancel={() => setEditing(null)} onSave={patch} />
            ) : (
              <>
                <h4 className="workflow-page__label">{t('workflow.page.languageCurrency')}</h4>
                <p className="workflow-page__chips">
                  <span className="workflow-page__chip workflow-page__chip--accent">{(w.language ?? '').toUpperCase()}</span>
                  <span className="workflow-page__chip">{w.currency}</span>
                  <span>{w.prices_are_incl_tax ? t('workflow.page.inclTax') : t('workflow.page.exclTax')}</span>
                </p>
                {w.default && <p className="hint">{t('workflow.page.isDefault')}</p>}
                {!w.active && <p className="hint">{t('workflow.page.inactive')}</p>}
                {invoices && (
                  <>
                    <h4 className="workflow-page__label">{t('workflow.page.titles')}</h4>
                    <p>{dt('workflow.page.invoiceTitle')} • {dt('workflow.page.creditTitle')}</p>
                    <NotAvailable />
                    <h4 className="workflow-page__label">{t('workflow.page.attachments')}</h4>
                    <p>{t('workflow.page.noAttachments')}</p>
                    <NotAvailable />
                    <h4 className="workflow-page__label">{t('workflow.page.onlineViewing')}</h4>
                    <p>{t('workflow.page.notViewable')}</p>
                    <NotAvailable />
                  </>
                )}
              </>
            )}
          </Card>
          {invoices && (
            <Card title={t('workflow.page.payment')} editLabel={t('workflow.page.editPayment')} editing={editing === 'payment'} onEdit={() => open('payment')}>
              {editing === 'payment' ? (
                <PaymentForm workflow={w} pending={save.isPending} errors={errors} onCancel={() => setEditing(null)} onSave={patch} />
              ) : (
                <>
                  <h4 className="workflow-page__label">{t('workflow.page.directDebit')}</h4>
                  <p>{t('workflow.page.noDirectDebit')}</p>
                  <NotAvailable />
                  <h4 className="workflow-page__label">{t('workflow.page.onlineMethods')}</h4>
                  {w.show_qr_code ? <p className="workflow-page__chip">{t('workflow.page.qrCode')}</p> : <p>{t('workflow.page.noMethods')}</p>}
                </>
              )}
            </Card>
          )}
        </div>

        <div className="workflow-page__main">
          {invoices && (
            <div className="workflow-tabs" role="tablist" aria-label={t('workflow.page.tabsLabel')}>
              {(['sending', 'payment'] as const).map((k) => (
                <button key={k} type="button" role="tab" id={`workflow-tab-${k}`} aria-selected={tab === k} aria-controls={`workflow-panel-${k}`} className="workflow-tabs__tab" onClick={() => setTab(k)}>
                  {t(`workflow.page.tabs.${k}`)}
                </button>
              ))}
            </div>
          )}
          {tab === 'sending' || !invoices ? (
            <div id="workflow-panel-sending" role={invoices ? 'tabpanel' : undefined} aria-labelledby={invoices ? 'workflow-tab-sending' : undefined} className="workflow-page__steps">
              <StepCard
                title={invoices ? t('workflow.page.sendInvoice') : t('workflow.page.sendQuote')}
                day={1}
                open
                editLabel={t('workflow.page.edit', { name: invoices ? t('workflow.page.sendInvoice') : t('workflow.page.sendQuote') })}
                editing={editing === 'send'}
                onEdit={() => open('send')}
              >
                {editing === 'send' ? (
                  <SendForm
                    workflow={w}
                    term={term}
                    invoices={invoices}
                    pending={save.isPending}
                    errors={errors}
                    extraTags={(sender?.custom_fields ?? []).map((f) => ({ group: 'company' as const, name: `custom_field_${f.id}`, label: f.name ?? '' }))}
                    onCancel={() => setEditing(null)}
                    onSave={(c, newTerm) => patch({ ...c, steps: [{ ...steps[0]!, due_interval: newTerm }, ...reminders] })}
                  />
                ) : (
                  <>
                    <p>{invoices ? t('workflow.page.sendIntro') : t('workflow.page.sendQuoteIntro')}</p>
                    <hr className="workflow-page__rule" />
                    <h4 className="workflow-page__label">{invoices ? t('workflow.page.expiresAfter') : t('workflow.page.validFor')}</h4>
                    <p>{t('workflow.page.days', { count: term })}</p>
                    {invoices && (
                      <>
                        <h4 className="workflow-page__label">{t('workflow.page.conditions')}</h4>
                        <div className="workflow-page__preview">
                          {w.payment_conditions ? <p><TextileText text={w.payment_conditions} values={values} /></p> : <p className="hint">{t('workflow.page.noConditions')}</p>}
                        </div>
                        <h4 className="workflow-page__label">{t('workflow.page.invoiceEmail')}</h4>
                        <div className="workflow-page__preview">
                          {w.invoice_email_subject && <p><strong>{email(w.invoice_email_subject)}</strong></p>}
                          <Paragraphs text={email(w.invoice_email_body || dt('invoices.sendForm.defaultMessage'))} />
                        </div>
                      </>
                    )}
                  </>
                )}
              </StepCard>

              {invoices && [...reminders, ...(isNew(editing, reminders.length) ? [{ due_interval: 0, reminder_delay: 7, reminder_text: '', reminder_auto_send: false }] : [])].map((r, i) => {
                const name = reminderName(i)
                const next = reminders[i + 1]
                const thisOne = typeof editing === 'object' && editing?.reminder === i
                return (
                  <StepCard key={i} title={name} day={days[i] ?? (days.at(-1) ?? 1 + term) + r.reminder_delay} open={thisOne} editLabel={t('workflow.page.edit', { name })} editing={thisOne} onEdit={() => open({ reminder: i })}>
                    {thisOne ? (
                      <ReminderForm
                        reminder={r}
                        name={name}
                        removeLabel={t('workflow.removeReminder', { n: i + 1 })}
                        pending={save.isPending}
                        errors={errors}
                        onCancel={() => setEditing(null)}
                        onRemove={i < reminders.length ? () => patch({ steps: steps.filter((_, j) => j !== i + 1) }) : undefined}
                        onSave={(changed) => patch({ steps: [steps[0]!, ...reminders.slice(0, i), changed, ...reminders.slice(i + 1)] })}
                      />
                    ) : (
                      <>
                        <p>{intros[i] ?? t('workflow.page.laterIntro')}</p>
                        <hr className="workflow-page__rule" />
                        <h4 className="workflow-page__label">{t('workflow.page.reminderSending')}</h4>
                        <p>{r.reminder_auto_send ? t('workflow.page.automatic') : t('workflow.page.manual')}</p>
                        <h4 className="workflow-page__label">{t('workflow.page.reminderEmail')}</h4>
                        <div className="workflow-page__preview">
                          <Paragraphs text={email(r.reminder_text)} />
                          <p className="hint">{t('workflow.page.unpaidTable')}</p>
                        </div>
                        {next && (
                          <>
                            <h4 className="workflow-page__label">{t('workflow.page.expiresAfter')}</h4>
                            <p>{t('workflow.page.days', { count: next.reminder_delay })}</p>
                          </>
                        )}
                      </>
                    )}
                  </StepCard>
                )
              })}
              {invoices && reminders.length < 10 && !isNew(editing, reminders.length) && (
                <button type="button" className="icon-link icon-link--button" onClick={() => open({ reminder: reminders.length })}>{t('workflow.addReminder')}</button>
              )}
            </div>
          ) : (
            <div id="workflow-panel-payment" role="tabpanel" aria-labelledby="workflow-tab-payment" className="workflow-page__steps">
              <StepCard title={t('workflow.page.paymentSucceeded')} open>
                <p>{t('workflow.page.paymentSucceededIntro')}</p>
                <hr className="workflow-page__rule" />
                <h4 className="workflow-page__label">{t('workflow.page.sendAfterPayment')}</h4>
                <p>{t('workflow.page.noEmailAfterPayment')}</p>
                <h4 className="workflow-page__label">{t('workflow.page.succeededLink')}</h4>
                <p>-</p>
                <NotAvailable />
              </StepCard>
              <StepCard title={t('workflow.page.paymentFailed')} open>
                <p>{t('workflow.page.paymentFailedIntro')}</p>
                <hr className="workflow-page__rule" />
                <h4 className="workflow-page__label">{t('workflow.page.sendOnFailure')}</h4>
                <p>{t('workflow.page.noFailureEmails')}</p>
                <NotAvailable />
              </StepCard>
              <StepCard title={t('workflow.page.changedIban')} open>
                <p>{t('workflow.page.changedIbanIntro')}</p>
                <NotAvailable />
              </StepCard>
            </div>
          )}
        </div>
      </div>
    </Page>
  )
}

const isNew = (editing: Editing, count: number) => typeof editing === 'object' && editing?.reminder === count

function NotAvailable() {
  const { t } = useTranslation()
  return <p className="workflow-page__unavailable">{t('workflow.page.notAvailable')}</p>
}

/** A text as paragraphs, split on blank lines, with its line breaks kept. */
function Paragraphs({ text }: { text: string }) {
  return (
    <>
      {text.split(/\n{2,}/).map((p, i) => (
        <p key={i} className="workflow-page__paragraph">{p}</p>
      ))}
    </>
  )
}

/** A card of the left column, with its edit button on the right of its title. */
function Card({ title, editLabel, editing, onEdit, children }: { title: string; editLabel: string; editing: boolean; onEdit: () => void; children: ReactNode }) {
  const heading = useId()
  return (
    <section className="workflow-card" aria-labelledby={heading}>
      <header className="workflow-card__header">
        <h3 id={heading} className="workflow-card__title">{title}</h3>
        {!editing && (
          <button type="button" className="btn btn--secondary btn--small icon-only" aria-label={editLabel} title={editLabel} onClick={onEdit}>
            <ListIcon kind="edit" />
          </button>
        )}
      </header>
      {children}
    </section>
  )
}

type StepProps = { title: string; day?: number; open?: boolean; editLabel?: string; editing?: boolean; onEdit?: () => void; children: ReactNode }

/** A step of the workflow, with its day, folded open or shut with Show more and Show less as on Moneybird. */
function StepCard({ title, day, open = false, editLabel, editing = false, onEdit, children }: StepProps) {
  const { t } = useTranslation()
  const [shown, setShown] = useState(open)
  const heading = useId()
  const body = useId()
  const expanded = shown || editing
  return (
    <section className="workflow-card workflow-card--step" aria-labelledby={heading}>
      <header className="workflow-card__header">
        <h3 id={heading} className="workflow-card__title">
          {title}
          {day !== undefined && <span className="workflow-card__day">{t('workflow.page.day', { n: day })}</span>}
        </h3>
        <div className="workflow-card__actions">
          {!editing && (
            <button type="button" className="btn btn--secondary btn--small" aria-expanded={expanded} aria-controls={body} onClick={() => setShown(!shown)}>
              {expanded ? t('workflow.page.showLess') : t('workflow.page.showMore')}
            </button>
          )}
          {onEdit && !editing && (
            <button type="button" className="btn btn--secondary btn--small icon-only" aria-label={editLabel} title={editLabel} onClick={onEdit}>
              <ListIcon kind="edit" />
            </button>
          )}
        </div>
      </header>
      {expanded && <div id={body} className="workflow-card__body">{children}</div>}
    </section>
  )
}

type FormErrors = ReturnType<typeof formErrors>

function Actions({ pending, onCancel }: { pending: boolean; onCancel: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="form-sheet__actions">
      <button className="btn btn--primary" disabled={pending}>{t('settings.save')}</button>
      <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
    </div>
  )
}

type GeneralProps = { workflow: Workflow; administration: Administration; errors: FormErrors; pending: boolean; onCancel: () => void; onSave: (c: Partial<Changes>) => void }

function GeneralForm({ workflow: w, administration, errors, pending, onCancel, onSave }: GeneralProps) {
  const { t, i18n } = useTranslation()
  const options = useOnboardingOptions()
  const [form, setForm] = useState({
    name: w.name ?? '', language: w.language ?? administration.language, currency: w.currency ?? administration.currency,
    prices_are_incl_tax: w.prices_are_incl_tax ?? false, active: w.active ?? true, default: w.default ?? false,
  })
  const currencyOptions = useMemo(() => {
    const names = new Intl.DisplayNames([i18n.language], { type: 'currency', fallback: 'code' })
    return (options.data?.currencies ?? [form.currency]).map((value) => ({ value, label: `${value} - ${names.of(value) ?? value}` }))
  }, [options.data, form.currency, i18n.language])
  const submit = (e: FormEvent) => {
    e.preventDefault()
    onSave({
      name: form.name, language: form.language as 'en', currency: form.currency, prices_are_incl_tax: form.prices_are_incl_tax,
      ...(w.default ? {} : { active: form.active, default: form.default }),
    })
  }
  return (
    <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate aria-label={t('workflow.page.general')}>
      <Problems messages={errors.general} />
      <TextInput label={t('workflow.name')} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
      <SelectInput label={t('workflow.language')} width="medium" value={form.language} errors={errors.field('language')} onChange={(e) => setForm({ ...form, language: e.target.value })}>
        <option value="nl">Nederlands</option>
        <option value="nl-be">Nederlands (België)</option>
        <option value="en">English</option>
      </SelectInput>
      <SearchableSelect
        label={t('workflow.currency')}
        searchLabel={t('onboarding.findCurrency')}
        emptyLabel={t('onboarding.noOption')}
        width="medium"
        value={form.currency}
        options={currencyOptions}
        errors={errors.field('currency')}
        onChange={(currency) => setForm({ ...form, currency })}
      />
      <CheckboxInput label={t('workflow.pricesInclTax')} checked={form.prices_are_incl_tax} onChange={(e) => setForm({ ...form, prices_are_incl_tax: e.target.checked })} />
      {w.default ? (
        <p className="hint">{t('workflow.defaultLocked')}</p>
      ) : (
        <>
          <CheckboxInput label={t('workflow.active')} checked={form.active} errors={errors.field('active')} onChange={(e) => setForm({ ...form, active: e.target.checked, default: e.target.checked && form.default })} />
          <CheckboxInput label={t('workflow.default')} checked={form.default} disabled={!form.active} onChange={(e) => setForm({ ...form, default: e.target.checked })} />
        </>
      )}
      <Actions pending={pending} onCancel={onCancel} />
    </form>
  )
}

function PaymentForm({ workflow: w, errors, pending, onCancel, onSave }: { workflow: Workflow; errors: FormErrors; pending: boolean; onCancel: () => void; onSave: (c: Partial<Changes>) => void }) {
  const { t } = useTranslation()
  const [qr, setQr] = useState(w.show_qr_code ?? true)
  return (
    <form
      className="form-sheet form-sheet--block"
      noValidate
      aria-label={t('workflow.page.payment')}
      onSubmit={(e) => {
        e.preventDefault()
        onSave({ show_qr_code: qr })
      }}
    >
      <Problems messages={errors.general} />
      <CheckboxInput label={t('workflow.showQrCode')} checked={qr} onChange={(e) => setQr(e.target.checked)} />
      <Actions pending={pending} onCancel={onCancel} />
    </form>
  )
}

type SendProps = {
  workflow: Workflow; term: number; invoices: boolean; errors: FormErrors; pending: boolean
  extraTags: { group: 'company'; name: string; label: string }[]
  onCancel: () => void; onSave: (c: Partial<Changes>, term: number) => void
}

function SendForm({ workflow: w, term: initialTerm, invoices, errors, pending, extraTags, onCancel, onSave }: SendProps) {
  const { t } = useTranslation()
  const [term, setTerm] = useState(String(initialTerm))
  const [conditions, setConditions] = useState(w.payment_conditions ?? '')
  const [subject, setSubject] = useState(w.invoice_email_subject ?? '')
  const [body, setBody] = useState(w.invoice_email_body ?? '')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    onSave(invoices ? { payment_conditions: conditions.trim(), invoice_email_subject: subject.trim(), invoice_email_body: body } : {}, Number(term))
  }
  return (
    <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate aria-label={invoices ? t('workflow.page.sendInvoice') : t('workflow.page.sendQuote')}>
      <Problems messages={errors.general} />
      <TextInput
        label={invoices ? t('workflow.term') : t('workflow.validity')}
        type="number"
        inputMode="numeric"
        min={0}
        max={365}
        width="small"
        help={t('workflow.days')}
        value={term}
        errors={errors.field('steps')}
        onChange={(e) => setTerm(e.target.value)}
      />
      {invoices && (
        <>
          <TextileEditor label={t('workflow.page.conditions')} value={conditions} onChange={setConditions} maxLength={1000} extraTags={extraTags} errors={errors.field('payment_conditions')} />
          <p className="form-input__message form-input__message--help">{t('workflow.conditionsHelp')}</p>
          <TextInput label={t('workflow.page.emailSubject')} maxLength={255} autoComplete="off" help={t('workflow.emailSubjectHelp')} value={subject} errors={errors.field('invoice_email_subject')} onChange={(e) => setSubject(e.target.value)} />
          <TextArea label={t('workflow.page.invoiceEmail')} rows={6} maxLength={10000} help={t('workflow.emailBodyHelp')} value={body} errors={errors.field('invoice_email_body')} onChange={(e) => setBody(e.target.value)} />
        </>
      )}
      <Actions pending={pending} onCancel={onCancel} />
    </form>
  )
}

type ReminderProps = { reminder: Step; name: string; removeLabel: string; errors: FormErrors; pending: boolean; onCancel: () => void; onRemove?: () => void; onSave: (s: Step) => void }

function ReminderForm({ reminder, name, removeLabel, errors, pending, onCancel, onRemove, onSave }: ReminderProps) {
  const { t } = useTranslation()
  const [form, setForm] = useState({ ...reminder, delay: String(reminder.reminder_delay) })
  return (
    <form
      className="form-sheet form-sheet--block"
      noValidate
      aria-label={name}
      onSubmit={(e) => {
        e.preventDefault()
        onSave({ due_interval: form.due_interval, reminder_delay: Number(form.delay), reminder_text: form.reminder_text, reminder_auto_send: form.reminder_auto_send })
      }}
    >
      <Problems messages={[...errors.general, ...(errors.field('steps') ?? [])]} />
      <RadioRow
        label={t('workflow.page.reminderSending')}
        value={form.reminder_auto_send ? 'automatic' : 'manual'}
        options={[{ value: 'manual', label: t('workflow.page.manual') }, { value: 'automatic', label: t('workflow.page.automatic') }]}
        onChange={(v) => setForm({ ...form, reminder_auto_send: v === 'automatic' })}
      />
      <TextInput label={t('workflow.page.sentAfter')} type="number" inputMode="numeric" min={1} max={365} width="small" help={t('workflow.page.sentAfterHelp')} value={form.delay} onChange={(e) => setForm({ ...form, delay: e.target.value })} />
      <TextArea label={t('workflow.page.reminderEmail')} rows={6} maxLength={5000} value={form.reminder_text} onChange={(e) => setForm({ ...form, reminder_text: e.target.value })} />
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={pending}>{t('settings.save')}</button>
        <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
        {onRemove && <button type="button" className="btn btn--secondary" disabled={pending} onClick={onRemove}>{removeLabel}</button>}
      </div>
    </form>
  )
}
