import { type FocusEvent, type KeyboardEvent, type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { contactName } from '../contacts/queries'
import { isDay } from '../forms/calendar'
import { DateInput } from '../forms/DatePicker'
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

/**
 * The value being typed in a field and how it is saved: it follows the saved
 * value whenever that changes, and a value is sent once, unless sending it
 * went wrong.
 */
function useInlineValue(saved: string, saving: FieldSaving) {
  const [draft, setDraft] = useState(saved)
  const [seen, setSeen] = useState(saved)
  const [sent, setSent] = useState<string | null>(null)
  if (seen !== saved) {
    setSeen(saved)
    setDraft(saved)
    setSent(null)
  }
  const commit = (value: string = draft) => {
    const v = value.trim()
    if (v === saved || saving.pending || (v === sent && !saving.errors)) {
      return
    }
    setSent(v)
    saving.save(v)
  }
  return { draft, setDraft, commit }
}

type CommitProps = { pending: boolean; onCommit: () => void; children: ReactNode }

/** A field's row that saves when the focus leaves it or Enter is pressed in it, and looks busy while it saves. */
function Commit({ pending, onCommit, children }: CommitProps) {
  const leave = (e: FocusEvent<HTMLDivElement>) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
      onCommit()
    }
  }
  const enter = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' && (e.target as HTMLElement).tagName === 'INPUT') {
      e.preventDefault()
      onCommit()
    }
  }
  return (
    <div className={`inline-field${pending ? ' is-saving' : ''}`} onBlur={leave} onKeyDown={enter}>
      {children}
    </div>
  )
}

type TextProps = { label: string; saved: string; saving: FieldSaving; remark?: Confidence; maxLength?: number }

/** A text field of a document that saves itself when it is left, not on every key. */
export function InlineText({ label, saved, saving, remark, maxLength = 255 }: TextProps) {
  const { draft, setDraft, commit } = useInlineValue(saved, saving)
  return (
    <Commit pending={saving.pending} onCommit={() => commit()}>
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

type DateProps = {
  label: string
  saved: string
  saving: FieldSaving
  remark?: Confidence
  /** The day a number of days counts from, as the due date's "days" after the date. */
  daysFrom?: string
}

/**
 * A date of a document: a day picked or typed in full saves at once; anything
 * else, such as an emptied due date, when the field is left. A due date can
 * also be given as a number of days after the date.
 */
export function InlineDate({ label, saved, saving, remark, daysFrom }: DateProps) {
  const { t } = useTranslation()
  const { draft, setDraft, commit } = useInlineValue(saved, saving)
  const [days, setDays] = useState('')
  const changeDays = (value: string) => {
    setDays(value)
    if (daysFrom && /^\d{1,4}$/.test(value.trim())) {
      setDraft(addDays(daysFrom, Number(value)))
    }
  }
  return (
    <Commit pending={saving.pending} onCommit={() => commit()}>
      <DateInput
        label={label}
        remark={remark}
        value={draft}
        errors={saving.errors}
        onChange={(value) => {
          setDraft(value)
          setDays('')
          if (isDay(value)) {
            commit(value)
          }
        }}
        action={daysFrom !== undefined && (
          <span className="incoming-card__days">
            <input type="text" inputMode="numeric" aria-label={t('incoming.view.days')} maxLength={4} value={days} onChange={(e) => changeDays(e.target.value)} />
            <span aria-hidden="true">{t('incoming.view.days')}</span>
          </span>
        )}
      />
    </Commit>
  )
}

type ContactProps = { administrationId: string; name: string; saving: FieldSaving; remark?: Confidence }

/**
 * The contact of a document, after Moneybird: its name as plain text, which
 * opens a search of the contacts under it; the contact picked saves at once.
 */
export function InlineContact({ administrationId, name, saving, remark }: ContactProps) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const administration = useAdministration(administrationId)
  const [shown, setShown] = useState(name)
  const [seen, setSeen] = useState(name)
  if (seen !== name) {
    setSeen(name)
    setShown(name)
  }
  return (
    <div className={`inline-field${saving.pending ? ' is-saving' : ''}`}>
      <ContactSelect
        plain={{ searchPlaceholder: t('incoming.view.searchContact'), addLabel: t('incoming.view.newContact') }}
        busy={saving.pending}
        remark={remark}
        administrationId={administrationId}
        value={shown}
        errors={saving.errors}
        homeCountry={administration.data?.country ?? 'NL'}
        returnTo={pathname}
        onChange={(c) => {
          setShown(contactName(c))
          saving.save(String(c.id ?? ''))
        }}
      />
    </div>
  )
}
