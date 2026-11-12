import { type FormEvent, type KeyboardEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { useCurrentUser } from '../auth/queries'
import { formatAmount } from '../banking/queries'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors } from '../forms/Form'
import { CaretIcon } from '../layout/icons'
import { useUsers } from '../management/queries'
import { byDay, eventText, type HistoryItem, historyOf } from './history'
import { type SalesInvoice, useAddInvoiceNote, useDeleteInvoiceNote } from './queries'

type Props = { administrationId: string; invoice: SalesInvoice }

/** The people of the administration by id, as far as the user may see them; the user is always known. */
function usePeople(administrationId: string) {
  const users = useUsers(administrationId)
  const me = useCurrentUser()
  const people = new Map<string, string>()
  for (const u of users.data ?? []) {
    if (u.id && u.name) {
      people.set(String(u.id), u.name)
    }
  }
  if (me.data?.id && !people.has(String(me.data.id))) {
    people.set(String(me.data.id), me.data.name ?? me.data.email ?? '')
  }
  return people
}

/**
 * Moneybird's "Notes and tasks" beside a document: a field to add a note or
 * to-do, and what happened to the invoice by day.
 */
export function InvoiceHistory({ administrationId, invoice }: Props) {
  const { t, i18n } = useTranslation()
  const heading = useId()
  const people = usePeople(administrationId)
  const days = byDay(historyOf(invoice), new Date())
  const remove = useDeleteInvoiceNote(administrationId, invoice.id ?? '')
  const dayName = (label: string) =>
    label === 'today' || label === 'yesterday'
      ? t(`invoices.view.notes.${label}`)
      : new Date(`${label}T00:00:00`).toLocaleDateString(i18n.language, { dateStyle: 'long' })
  return (
    <section className="invoice-history" aria-labelledby={heading}>
      <h2 id={heading} className="invoice-history__title">{t('invoices.view.notes.title')}</h2>
      <NoteForm administrationId={administrationId} invoice={invoice} people={people} />
      <Problems messages={formErrors(remove.error, {}).general} />
      {days.map((d) => (
        <div key={d.label} className="history">
          <h3 className="history__day">{dayName(d.label)}</h3>
          <ul className="history__list">
            {d.items.map((item) => (
              <HistoryRow
                key={item.id}
                item={item}
                invoice={invoice}
                person={item.userId ? people.get(item.userId) : undefined}
                removing={remove.isPending}
                onRemoveNote={(id) => remove.mutate(id)}
              />
            ))}
          </ul>
        </div>
      ))}
    </section>
  )
}

type RowProps = { item: HistoryItem; invoice: SalesInvoice; person?: string; removing: boolean; onRemoveNote: (id: string) => void }

function HistoryRow({ item, invoice, person, removing, onRemoveNote }: RowProps) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const details = useId()
  const time = new Date(item.at).toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
  const hasDetails = !!person || item.kind === 'note'
  return (
    <li className={`history__row history__row--${item.kind}`}>
      <span className="history__avatar" title={person} aria-hidden={!person}>{person ? person.trim().charAt(0).toUpperCase() : ''}</span>
      <p className="history__text"><Sentence item={item} invoice={invoice} /></p>
      {item.timed ? <time className="history__time" dateTime={item.at}>{time}</time> : <span className="history__time" />}
      {hasDetails && (
        <button type="button" className="history__caret" aria-label={t('invoices.view.notes.details')} title={t('invoices.view.notes.details')} aria-expanded={open} aria-controls={details} onClick={() => setOpen(!open)}>
          <CaretIcon />
        </button>
      )}
      {open && (
        <div id={details} className="history__details">
          {person && <p>{t('invoices.view.notes.by')} <strong>{person}</strong></p>}
          {item.kind === 'note' && (
            <ConfirmDelete
              label={t('invoices.view.notes.deleteNote')}
              confirmLabel={t('invoices.view.notes.confirmDeleteNote')}
              text={t('invoices.view.notes.delete')}
              confirmText={t('invoices.view.notes.confirmDelete')}
              disabled={removing}
              onConfirm={() => onRemoveNote(item.note.id ?? '')}
            />
          )}
        </div>
      )}
    </li>
  )
}

function Sentence({ item, invoice }: { item: HistoryItem; invoice: SalesInvoice }) {
  const { t, i18n } = useTranslation()
  if (item.kind === 'note') {
    const n = item.note
    return (
      <>
        <span className="history__note">{n.note}</span>
        {(n.todo === true || n.todo === 'true') && (
          <span className="history__todo">{n.completed_at ? t('invoices.view.notes.done') : t('invoices.view.notes.todo')}</span>
        )}
      </>
    )
  }
  if (item.kind === 'payment') {
    const p = item.payment
    const action = p.manual_payment_action
    return (
      <>
        <span>{t('invoices.view.events.payment', { amount: formatAmount(p.price, invoice.currency ?? 'EUR', i18n.language) })}</span>
        {!p.financial_mutation_id && (!action || action === 'bank_transfer') && <><br /><span className="history__quiet">{t('invoices.view.events.notLinked')}</span></>}
        {!p.financial_mutation_id && action && action !== 'bank_transfer' && (
          <><br /><span className="history__quiet">{t(`invoices.payments.actions.${action}`, { defaultValue: action })}</span></>
        )}
      </>
    )
  }
  const { key, state, action } = eventText(item.action)
  const stateName = state ? t(`invoices.states.${state}`, { defaultValue: state }).toLowerCase() : undefined
  return <span>{t(`invoices.view.events.${key}`, { state: stateName, action })}</span>
}

/** Note, to-do, or a to-do for someone, as Moneybird's select under the field. */
type Kind = 'note' | 'todo' | `user:${string}`

function NoteForm({ administrationId, invoice, people }: Props & { people: Map<string, string> }) {
  const { t } = useTranslation()
  const id = useId()
  const [text, setText] = useState('')
  const [kind, setKind] = useState<Kind>('note')
  const [focused, setFocused] = useState(false)
  const add = useAddInvoiceNote(administrationId, invoice.id ?? '')
  const errors = formErrors(add.error, { note: t('invoices.view.notes.label') })
  const expanded = focused || text !== ''

  const save = () => {
    if (!text.trim() || add.isPending) {
      return
    }
    const assignee = kind.startsWith('user:') ? { assignee_id: kind.slice(5) } : {}
    add.mutate({ note: text.trim(), todo: kind !== 'note', ...assignee }, {
      onSuccess: () => {
        setText('')
        setKind('note')
      },
    })
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save()
  }
  // Enter saves, as on Moneybird; Shift+Enter starts a new line.
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      save()
    }
  }

  return (
    <form
      className={`note-form${expanded ? ' is-expanded' : ''}`}
      onSubmit={submit}
      noValidate
      onFocus={() => setFocused(true)}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          setFocused(false)
        }
      }}
    >
      <Problems messages={errors.general} />
      <label className="sr-only" htmlFor={`${id}-note`}>{t('invoices.view.notes.label')}</label>
      <textarea
        id={`${id}-note`}
        className="note-form__text"
        rows={expanded ? 3 : 1}
        maxLength={10000}
        placeholder={t('invoices.view.notes.placeholder')}
        value={text}
        aria-invalid={errors.field('note') ? true : undefined}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
      />
      {errors.field('note') && <p className="form-input__message form-input__message--error">{errors.field('note')?.join(' ')}</p>}
      {expanded && (
        <div className="note-form__footer">
          <select aria-label={t('invoices.view.notes.kind')} value={kind} onChange={(e) => setKind(e.target.value as Kind)}>
            <option value="note">{t('invoices.view.notes.note')}</option>
            <option value="todo">{t('invoices.view.notes.todo')}</option>
            {[...people].map(([userId, name]) => <option key={userId} value={`user:${userId}`}>{t('invoices.view.notes.todoFor', { name })}</option>)}
          </select>
          <button className="btn btn--primary" disabled={add.isPending || !text.trim()}>{t('invoices.view.notes.save')}</button>
        </div>
      )}
    </form>
  )
}
