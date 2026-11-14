import { type FocusEvent, type KeyboardEvent, type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { contactName } from '../contacts/queries'
import { isDay } from '../forms/calendar'
import { DateInput } from '../forms/DatePicker'
import { type DueMode, DueModeToggle } from '../forms/DueModeToggle'
import { formErrors, TextInput } from '../forms/Form'
import type { Confidence } from '../forms/Remark'
import { ContactSelect } from '../invoices/ContactSelect'
import { addDays } from '../invoices/paper'

/** How one field of a document saves itself: its own request, whether it is under way, and what the server said of it. */
export type FieldSaving = { save: (value: string) => void; pending: boolean; errors?: string[] }

/** What a save of one field went wrong with: the problems of that field and those of no field, shown under it. */
export function fieldErrors(error: unknown, name: string, label: string): string[] | undefined {
  const errors = formErrors(error, { [name]: label })
  const problems = [...(errors.field(name) ?? []), ...errors.general]
  return problems.length > 0 ? problems : undefined
}

/** Why a field saves: it was left, or Enter was pressed or a day picked in it. */
type Reason = 'leave' | 'enter'

/**
 * The value being typed in a field and how it is saved: it follows the saved
 * value whenever that changes, and a value is sent once, unless sending it
 * went wrong. A field Scan and Recognise filled in (confirm) is also saved
 * unchanged, when Enter is pressed in it or it is left after typing, so the
 * user confirms what was read.
 */
function useInlineValue(saved: string, saving: FieldSaving, confirm = false) {
  const [draft, setDraftValue] = useState(saved)
  const [seen, setSeen] = useState(saved)
  const [sent, setSent] = useState<string | null>(null)
  const [typed, setTyped] = useState(false)
  if (seen !== saved) {
    setSeen(saved)
    setDraftValue(saved)
    setSent(null)
    setTyped(false)
  }
  const setDraft = (value: string) => {
    setDraftValue(value)
    setTyped(true)
  }
  const commit = (value: string = draft, reason: Reason = 'enter') => {
    const v = value.trim()
    const confirming = confirm && (reason === 'enter' || typed)
    if ((v === saved && !confirming) || saving.pending || (v === sent && !saving.errors)) {
      return
    }
    setSent(v)
    saving.save(v)
  }
  return { draft, setDraft, commit }
}

type CommitProps = { pending: boolean; onCommit: (reason: Reason) => void; children: ReactNode; className?: string }

/** A field's row that saves when the focus leaves it or Enter is pressed in it, and looks busy while it saves. */
function Commit({ pending, onCommit, children, className }: CommitProps) {
  const leave = (e: FocusEvent<HTMLDivElement>) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
      onCommit('leave')
    }
  }
  const enter = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' && (e.target as HTMLElement).tagName === 'INPUT' && (e.target as HTMLInputElement).type !== 'radio') {
      e.preventDefault()
      onCommit('enter')
    }
  }
  return (
    <div className={`inline-field${pending ? ' is-saving' : ''}${className ? ` ${className}` : ''}`} onBlur={leave} onKeyDown={enter}>
      {children}
    </div>
  )
}

type TextProps = { label: string; saved: string; saving: FieldSaving; remark?: Confidence; maxLength?: number }

/** A text field of a document that saves itself when it is left, not on every key. */
export function InlineText({ label, saved, saving, remark, maxLength = 255 }: TextProps) {
  const { draft, setDraft, commit } = useInlineValue(saved, saving, remark !== undefined)
  return (
    <Commit pending={saving.pending} onCommit={(reason) => commit(draft, reason)}>
      <TextInput
        label={label}
        remark={remark}
        maxLength={maxLength}
        value={draft}
        errors={saving.errors}
        aria-busy={saving.pending || undefined}
        onChange={(e) => setDraft(e.target.value)}
      />
    </Commit>
  )
}

type DateProps = { label: string; saved: string; saving: FieldSaving; remark?: Confidence }

/**
 * A date of a document: a day picked or typed in full saves at once; anything
 * else, such as an emptied date, when the field is left.
 */
export function InlineDate({ label, saved, saving, remark }: DateProps) {
  const { draft, setDraft, commit } = useInlineValue(saved, saving, remark !== undefined)
  return (
    <Commit pending={saving.pending} onCommit={(reason) => commit(draft, reason)}>
      <DateInput
        label={label}
        remark={remark}
        value={draft}
        errors={saving.errors}
        onChange={(value) => {
          setDraft(value)
          if (isDay(value)) {
            commit(value)
          }
        }}
      />
    </Commit>
  )
}

/** The digits of a number of days, at most three, as Moneybird's "Number of days". */
const dayDigits = (value: string) => value.replace(/\D/g, '').slice(0, 3)

/**
 * The due date of a document, after Moneybird: one field, a date or a number
 * of days after the document's date, as the switch at its right chooses. A
 * day picked saves at once; days save when the field is left or Enter is
 * pressed in it, as the date they come to.
 */
export function InlineDueDate({ label, saved, saving, remark, from }: DateProps & { from: string }) {
  const { t } = useTranslation()
  const { draft, setDraft, commit } = useInlineValue(saved, saving, remark !== undefined)
  const [mode, setMode] = useState<DueMode>('date')
  const [days, setDays] = useState('')
  const toggle = (
    <DueModeToggle
      mode={mode}
      label={t('incoming.view.dueAs')}
      words={{ date: t('incoming.view.date'), days: t('incoming.view.days') }}
      onMode={(m) => {
        // The field in use goes away, so what it holds saves as when it is left.
        commit(draft, 'leave')
        setMode(m)
        setDays('')
      }}
    />
  )
  const changeDays = (value: string) => {
    const digits = dayDigits(value)
    setDays(digits)
    if (digits !== '' && isDay(from)) {
      setDraft(addDays(from, Number(digits)))
    }
  }
  return (
    <Commit className="inline-field--due" pending={saving.pending} onCommit={(reason) => commit(draft, reason)}>
      {mode === 'date'
        ? (
          <DateInput
            label={label}
            remark={remark}
            value={draft}
            errors={saving.errors}
            action={toggle}
            onChange={(value) => {
              setDraft(value)
              if (isDay(value)) {
                commit(value)
              }
            }}
          />
        )
        : (
          <TextInput
            label={label}
            aria-label={t('incoming.view.numberOfDays')}
            placeholder={t('incoming.view.numberOfDays')}
            remark={remark}
            width="medium"
            inputMode="numeric"
            maxLength={3}
            value={days}
            errors={saving.errors}
            action={toggle}
            onChange={(e) => changeDays(e.target.value)}
          />
        )}
    </Commit>
  )
}

type ContactProps = { administrationId: string; contactId?: string | number | null; name: string; saving: FieldSaving; remark?: Confidence }

/**
 * The contact of a document, after Moneybird: its name as plain text, which
 * opens a search of the contacts under it; the contact picked saves at once.
 * While the name is hovered or focused, the contact's card shows.
 */
export function InlineContact({ administrationId, contactId, name, saving, remark }: ContactProps) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const administration = useAdministration(administrationId)
  const [shown, setShown] = useState({ id: String(contactId ?? ''), name })
  const [seen, setSeen] = useState(name)
  if (seen !== name) {
    setSeen(name)
    setShown({ id: String(contactId ?? ''), name })
  }
  return (
    <div className={`inline-field${saving.pending ? ' is-saving' : ''}`}>
      <ContactSelect
        plain={{ searchPlaceholder: t('incoming.view.searchContact'), addLabel: t('incoming.view.newContact') }}
        busy={saving.pending}
        remark={remark}
        administrationId={administrationId}
        value={shown.name}
        hoverCard={shown.id && shown.name ? shown : undefined}
        errors={saving.errors}
        homeCountry={administration.data?.country ?? 'NL'}
        returnTo={pathname}
        onChange={(c) => {
          setShown({ id: String(c.id ?? ''), name: contactName(c) })
          saving.save(String(c.id ?? ''))
        }}
      />
    </div>
  )
}
