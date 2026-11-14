import { type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { contactName } from '../contacts/queries'
import { formErrors } from '../forms/Form'
import { ContactHoverCard } from '../contacts/ContactHoverCard'
import { type EditedInvoice, InvoiceEditor, type Opening } from '../invoices/InvoiceFormPage'
import { NotesAndHistory } from '../invoices/InvoiceHistory'
import { ChangeLayoutPanel, LayoutPicker } from '../invoices/LayoutPicker'
import { today } from '../invoices/paper'
import { useSalesInvoice } from '../invoices/queries'
import { useDocumentStyles, useWorkflows } from '../invoicing/queries'
import { Loading } from '../layout/Loading'
import { ConfirmDialog, DangerDialog } from '../layout/Modal'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { PageIcon } from '../layout/pageIcons'
import { ActionMenu, type MenuItem } from '../list/ListActions'
import {
  type RecurringInvoice, recurringHistory, recurringTitle, useAddRecurringNote, useDeactivateRecurringInvoice, useDeleteRecurringInvoice,
  useDeleteRecurringNote, useInvoicesOf, useRecurringInvoice, useSaveRecurringInvoice,
} from './queries'
import { draftOf, type ScheduleDraft, scheduleChanges, ScheduleForm, scheduleLabels, scheduleOfDraft, ScheduleSummary } from './Schedule'
import '../invoices/view.css'
import './recurring.css'

/**
 * One recurring invoice (/recurring_sales_invoices/:id), or a new one
 * (/recurring_sales_invoices/new; ?from= a sales invoice or ?recurring= a
 * recurring invoice to start from, as their "Duplicate to recurring invoice").
 */
export function RecurringInvoicePage() {
  const { administrationId = '', recurringId } = useParams()
  const [params] = useSearchParams()
  if (!recurringId) {
    return <NewRecurringInvoice key={params.toString()} administrationId={administrationId} from={params.get('from') ?? undefined} copy={params.get('recurring') ?? undefined} />
  }
  return <RecurringView key={recurringId} administrationId={administrationId} recurringId={recurringId} />
}

/** What a copy starts from: everything of the document, nothing that makes it the original. */
const templateOf = (source: EditedInvoice): EditedInvoice => ({ ...source, id: undefined, version: undefined })

/**
 * Moneybird's "Add recurring invoice": first its schedule over a preview of
 * the document, then, after Continue, the popup with the contact over the
 * document, whose Save makes the recurring invoice.
 */
function NewRecurringInvoice({ administrationId, from, copy }: { administrationId: string; from?: string; copy?: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const source = useSalesInvoice(administrationId, from)
  const original = useRecurringInvoice(administrationId, copy)
  const styles = useDocumentStyles(administrationId)
  const [draft, setDraft] = useState<ScheduleDraft>(() => draftOf(null, today()))
  const [step, setStep] = useState<'schedule' | 'document'>('schedule')
  const [layoutsOpen, setLayoutsOpen] = useState(false)
  const [styleId, setStyleId] = useState<string | undefined>(undefined)
  const layoutsId = useId()
  const list = `/${administrationId}/recurring_sales_invoices`
  const base = (from && source.data) || (copy && original.data) || null
  // The layouts are waited for by the editor below: waiting here as well would unmount it, and its queries would start again each time.
  if ((from && !source.data && !source.error) || (copy && !original.data && !original.error)) {
    return <Loading />
  }
  const allStyles = styles.data ?? []
  const chosenStyle = styleId ?? base?.document_style_id ?? (allStyles.find((s) => s.default) ?? allStyles[0])?.id ?? ''
  const template: EditedInvoice = { ...(base ? templateOf(base) : {}), document_style_id: styleId ?? base?.document_style_id }
  const errors = formErrors(null, scheduleLabels(t))

  return (
    <Page
      wide
      title={t('recurring.add')}
      actions={
        <div className="recurring-actions">
          {step === 'schedule' && (
            <button type="button" className="btn btn--secondary" aria-expanded={layoutsOpen} aria-controls={layoutsId} onClick={() => setLayoutsOpen(!layoutsOpen)}>
              {t('invoices.editor.changeLayout')}
            </button>
          )}
        </div>
      }
    >
      <div className="recurring-page recurring-page--new">
        {layoutsOpen && step === 'schedule' && (
          <LayoutPicker id={layoutsId} administrationId={administrationId} styles={allStyles} current={chosenStyle} onPick={setStyleId} />
        )}
        <section className="recurring-schedule" aria-label={t('recurring.schedule')}>
          {step === 'schedule' ? (
            <ScheduleForm
              value={draft}
              onChange={(c) => setDraft({ ...draft, ...c })}
              submitLabel={t('recurring.continue')}
              onSubmit={() => {
                setLayoutsOpen(false)
                setStep('document')
              }}
              errors={errors}
            />
          ) : (
            <ScheduleSummary schedule={scheduleOfDraft(draft)} active made={0} administrationId={administrationId} onEdit={() => setStep('schedule')} />
          )}
        </section>
        {/* The document is a preview until Continue; then its popup asks for the contact, as on Moneybird. A fresh editor per step takes the layout chosen. */}
        <div inert={step === 'schedule'} className={step === 'schedule' ? 'recurring-page__preview' : undefined}>
          <InvoiceEditor
            key={`${step}-${styleId ?? ''}`}
            mode="view"
            administrationId={administrationId}
            invoice={null}
            recurring={{ invoice: null, schedule: scheduleChanges(draft, null) }}
            template={template}
            opening={{ dialog: step === 'document' }}
            layoutsOpen={false}
            onLayoutsClose={() => setLayoutsOpen(false)}
            onCancel={() => setStep('schedule')}
            onSaved={(saved, opening) => navigate(`${list}/${saved.id}`, { state: opening })}
          />
        </div>
      </div>
    </Page>
  )
}

function RecurringView({ administrationId, recurringId }: { administrationId: string; recurringId: string }) {
  const recurring = useRecurringInvoice(administrationId, recurringId)
  if (recurring.error instanceof ApiError && recurring.error.status < 500) {
    return <NotFoundPage />
  }
  if (!recurring.data) {
    return <Loading />
  }
  return <RecurringScreen administrationId={administrationId} recurring={recurring.data} />
}

/**
 * A recurring invoice after Moneybird's page: "Every month to …" with Extra,
 * Stop and delete, the schedule's sentence and timeline above the document,
 * which is edited in place as a draft invoice is, and the sidebar with the
 * contact, workflow, notes and history.
 */
function RecurringScreen({ administrationId, recurring: rec }: { administrationId: string; recurring: RecurringInvoice }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const stop = useDeactivateRecurringInvoice(administrationId, rec.id ?? '')
  const remove = useDeleteRecurringInvoice(administrationId, rec.id ?? '')
  const save = useSaveRecurringInvoice(administrationId, rec.id)
  const made = useInvoicesOf(administrationId, rec.id)
  const styles = useDocumentStyles(administrationId)
  const [layoutsOpen, setLayoutsOpen] = useState(false)
  const layoutsId = useId()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(() => draftOf(rec, today()))
  const [asking, setAsking] = useState<'stop' | 'delete' | null>(null)
  const list = `/${administrationId}/recurring_sales_invoices`
  const here = `${list}/${rec.id}`
  const name = rec.contact ? contactName(rec.contact) : ''
  const active = !!rec.active
  const count = rec.invoices_created ?? 0

  const extra: MenuItem[] = [
    ...(active ? [{ label: t('invoices.view.changeLayout'), onSelect: () => setLayoutsOpen(true) }] : []),
    { label: t('invoices.view.duplicateRecurring'), to: `${list}/new?recurring=${rec.id}` },
  ]
  const actions = (
    <div className="recurring-actions">
      <ActionMenu label={t('invoices.view.extra')} items={extra} />
      {active && <button type="button" className="btn btn--secondary" disabled={stop.isPending} onClick={() => setAsking('stop')}>{t('recurring.stop')}</button>}
      <button type="button" className="btn btn--secondary btn--icon" aria-label={t('recurring.delete')} title={t('recurring.delete')} onClick={() => setAsking('delete')}>
        <PageIcon kind="trash" />
      </button>
    </div>
  )
  const editSchedule = () => {
    save.reset()
    setDraft(draftOf(rec, today()))
    setEditing(true)
  }
  const scheduleErrors = formErrors(save.error, scheduleLabels(t))

  return (
    <Page wide title={recurringTitle(rec, name, t)} actions={actions}>
      <div className="invoice-view recurring-page">
        <div className="invoice-view__main">
          <Problems messages={formErrors(stop.error ?? remove.error, {}).general} />
          {!active && <p className="notice">{t('recurring.inactiveNotice')}</p>}
          {/* As on Moneybird, Extra's Change layout opens above the schedule; the layout is saved on its own. */}
          {layoutsOpen && (
            <ChangeLayoutPanel
              id={layoutsId}
              administrationId={administrationId}
              styles={styles.data ?? []}
              current={rec.document_style_id ?? ''}
              text={t('recurring.layoutText')}
              saving={save.isPending}
              onPick={(id) => save.mutate({ document_style_id: id }, { onSuccess: () => setLayoutsOpen(false) })}
              onCancel={() => setLayoutsOpen(false)}
            />
          )}
          <section className="recurring-schedule" aria-label={t('recurring.schedule')}>
            {editing ? (
              <ScheduleForm
                value={draft}
                onChange={(c) => setDraft({ ...draft, ...c })}
                submitLabel={t('recurring.save')}
                pending={save.isPending}
                errors={scheduleErrors}
                onSubmit={() => save.mutate(scheduleChanges(draft, rec), { onSuccess: () => setEditing(false) })}
                onCancel={() => setEditing(false)}
              />
            ) : (
              <ScheduleSummary
                schedule={rec}
                active={active}
                made={count}
                invoices={made.data ?? []}
                administrationId={administrationId}
                onEdit={active ? editSchedule : undefined}
              />
            )}
          </section>
          {/* A stopped recurring invoice makes nothing more, so its document no longer changes. */}
          <div inert={!active}>
            <InvoiceEditor
              mode="view"
              administrationId={administrationId}
              invoice={null}
              recurring={{ invoice: rec }}
              opening={location.state as Opening}
              layoutsOpen={false}
              onLayoutsClose={() => setLayoutsOpen(false)}
              onCancel={() => undefined}
              onSaved={(_, opening) => navigate(here, { replace: true, state: opening })}
            />
          </div>
        </div>
        <RecurringSidebar administrationId={administrationId} recurring={rec} />
      </div>
      {asking === 'delete' && (
        <DangerDialog
          question={t('recurring.confirmDelete')}
          busy={remove.isPending}
          onCancel={() => setAsking(null)}
          // One that made invoices is stopped instead, as the server keeps what it made.
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => (count > 0 ? setAsking(null) : navigate(list)) })}
        />
      )}
      {asking === 'stop' && (
        <ConfirmDialog
          title={t('recurring.confirmStop')}
          description={t('recurring.confirmStopText')}
          confirmLabel={t('recurring.stop')}
          onCancel={() => setAsking(null)}
          onConfirm={() => stop.mutate(undefined, { onSuccess: () => setAsking(null) })}
        />
      )}
    </Page>
  )
}

/** Moneybird's column beside a recurring invoice: its contact and workflow, and its notes and history. */
function RecurringSidebar({ administrationId, recurring: rec }: { administrationId: string; recurring: RecurringInvoice }) {
  const { t } = useTranslation()
  const workflows = useWorkflows(administrationId)
  const add = useAddRecurringNote(administrationId, rec.id ?? '')
  const remove = useDeleteRecurringNote(administrationId, rec.id ?? '')
  const flow = workflows.data?.find((w) => w.id === rec.workflow_id)
  const facts: [string, ReactNode][] = [
    [t('invoices.view.contact'), rec.contact && <ContactHoverCard administrationId={administrationId} contact={{ ...rec.contact, id: rec.contact_id }} />],
    [t('invoices.workflow'), flow && <Link to={`/${administrationId}/workflows/${flow.id}`}>{flow.name}</Link>],
  ]
  return (
    <aside className="invoice-sidebar" aria-label={t('recurring.sidebar')}>
      <dl className="invoice-sidebar__facts">
        {facts.filter(([, value]) => value).map(([label, value]) => (
          <div key={label} className="invoice-sidebar__fact">
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <NotesAndHistory administrationId={administrationId} items={recurringHistory(rec)} currency={rec.currency ?? 'EUR'} add={add} remove={remove} />
    </aside>
  )
}
