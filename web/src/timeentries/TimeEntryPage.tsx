import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { useCurrentUser } from '../auth/queries'
import { Problems } from '../auth/AuthLayout'
import { ConfirmPanel, DetailBlock, DetailList, type DetailRow } from '../detail/Detail'
import { CheckboxInput, FormSection, formErrors, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type MenuItem, MoreMenu } from '../list/ListActions'
import { contactName, TimeBadge } from './TimeEntriesPage'
import { duration, type TimeEntry, useCreateTimeEntry, useDeleteTimeEntry, useTimeEntry, useTimer, workedMinutes } from './queries'

/** One time entry (/time_entries/:timeEntryId), or new time (/time_entries/new). */
export function TimeEntryPage() {
  const { t } = useTranslation()
  const { administrationId = '', timeEntryId } = useParams()
  const entry = useTimeEntry(administrationId, timeEntryId)
  if (timeEntryId && entry.error instanceof ApiError && entry.error.status < 500) {
    return <NotFoundPage />
  }
  if (!timeEntryId) {
    return (
      <Page title={t('timeEntries.newTitle')} back={{ to: `/${administrationId}/time_entries`, label: t('timeEntries.backToList') }}>
        <TimeEntryForm administrationId={administrationId} />
      </Page>
    )
  }
  if (!entry.data) {
    return <Loading />
  }
  return <TimeEntryView administrationId={administrationId} entry={entry.data} />
}

const clock = (iso: string | null | undefined) => (iso ? new Date(iso).toLocaleString() : '')

function TimeEntryView({ administrationId, entry }: { administrationId: string; entry: TimeEntry }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const timer = useTimer(administrationId, entry.id ?? '')
  const remove = useDeleteTimeEntry(administrationId, entry.id ?? '')
  const [confirming, setConfirming] = useState(false)
  const list = `/${administrationId}/time_entries`
  const running = !entry.ended_at

  const more: MenuItem[] = [
    running
      ? { label: t('timeEntries.stop'), disabled: timer.isPending, onSelect: () => timer.mutate('stop') }
      : { label: t('timeEntries.resume'), disabled: timer.isPending, onSelect: () => timer.mutate('resume') },
    ...(entry.sales_invoice_id ? [] : [{ label: t('timeEntries.delete'), danger: true, onSelect: () => setConfirming(true) }]),
  ]
  const rows: DetailRow[] = [
    [t('timeEntries.status'), <TimeBadge key="state" entry={entry} />],
    [t('timeEntries.user'), entry.user?.name ?? ''],
    [t('timeEntries.startedAt'), clock(entry.started_at)],
    [t('timeEntries.endedAt'), clock(entry.ended_at)],
    [t('timeEntries.duration'), duration(workedMinutes(entry))],
    [t('timeEntries.contact'), contactName(entry)],
    [t('timeEntries.project'), entry.project?.name ?? ''],
    [t('timeEntries.invoice'), entry.sales_invoice_id
      ? <Link to={`/${administrationId}/sales_invoices/${entry.sales_invoice_id}`}>{entry.sales_invoice?.invoice_id ?? t('timeEntries.draftInvoice')}</Link>
      : ''],
  ]

  return (
    <Page title={entry.description ?? ''} back={{ to: list, label: t('timeEntries.backToList') }} actions={<MoreMenu items={more} />}>
      <Problems messages={formErrors(remove.error ?? timer.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('timeEntries.confirmDeleteText')}
          confirmLabel={t('timeEntries.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      <DetailBlock title={t('timeEntries.detailsTitle')}>
        <DetailList rows={rows} />
      </DetailBlock>
    </Page>
  )
}

/** Local date and time for a datetime-local field. */
function localInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** New time of the signed-in user: what, from when to when, and whether to bill it. Without an end, a timer starts. */
function TimeEntryForm({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const me = useCurrentUser()
  const save = useCreateTimeEntry(administrationId)
  const [description, setDescription] = useState('')
  const [start, setStart] = useState(() => localInput(new Date()))
  const [end, setEnd] = useState('')
  const [billable, setBillable] = useState(true)
  const labels: Record<string, string> = { description: t('timeEntries.description'), started_at: t('timeEntries.startedAt'), ended_at: t('timeEntries.endedAt') }
  const errors = formErrors(save.error, labels)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate({
      user_id: me.data?.id ?? '', description: description.trim(), billable,
      started_at: new Date(start).toISOString(), ...(end ? { ended_at: new Date(end).toISOString() } : {}),
    }, { onSuccess: (saved) => navigate(`/${administrationId}/time_entries/${saved.id}`) })
  }

  return (
    <form className="form-sheet" onSubmit={submit} noValidate aria-label={t('timeEntries.form')}>
      <Problems messages={errors.general} />
      <FormSection>
        <TextInput label={labels.description ?? ''} required maxLength={10000} value={description} errors={errors.field('description')} onChange={(e) => setDescription(e.target.value)} />
        <TextInput label={labels.started_at ?? ''} type="datetime-local" required width="medium" value={start} errors={errors.field('started_at')} onChange={(e) => setStart(e.target.value)} />
        <TextInput label={labels.ended_at ?? ''} type="datetime-local" width="medium" help={t('timeEntries.endHelp')} value={end} errors={errors.field('ended_at')} onChange={(e) => setEnd(e.target.value)} />
        <CheckboxInput label={t('timeEntries.billable')} checked={billable} onChange={(e) => setBillable(e.target.checked)} />
      </FormSection>
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={save.isPending || !me.data}>{t('timeEntries.save')}</button>
        <Link className="btn btn--secondary" to={`/${administrationId}/time_entries`}>{t('common.cancel')}</Link>
      </div>
    </form>
  )
}
