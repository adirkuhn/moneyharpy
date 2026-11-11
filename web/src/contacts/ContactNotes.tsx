import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { DetailBlock } from '../detail/Detail'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, formErrors, TextArea } from '../forms/Form'
import { type Contact, useAddNote, useDeleteNote } from './queries'

/** The Tasks and notes tab of a contact: its notes and to-dos, and a form for a new one. */
export function ContactNotes({ administrationId, contact }: { administrationId: string; contact: Contact }) {
  const { t, i18n } = useTranslation()
  const [text, setText] = useState('')
  const [todo, setTodo] = useState(false)
  const add = useAddNote(administrationId, contact.id ?? '')
  const remove = useDeleteNote(administrationId, contact.id ?? '')
  const errors = formErrors(add.error ?? remove.error, { note: t('contacts.newNote') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    add.mutate({ note: text, todo }, { onSuccess: () => { setText(''); setTodo(false) } })
  }
  const notes = contact.notes ?? []
  return (
    <DetailBlock title={t('contacts.notes')}>
      {notes.length > 0 && (
        <ul className="notes">
          {notes.map((n) => (
            <li key={n.id} className="notes__row">
              <div>
                <p className="notes__text">{n.note}</p>
                <p className="hint">
                  {n.todo && <span className="ledger__state">{n.completed_at ? t('contacts.done') : t('contacts.todo')}</span>}{' '}
                  {new Date(n.created_at ?? '').toLocaleDateString(i18n.language, { dateStyle: 'medium' })}
                </p>
              </div>
              <ConfirmDelete label={t('contacts.deleteNote')} confirmLabel={t('contacts.confirmDeleteNote')} text={t('contacts.remove')} confirmText={t('contacts.confirmRemove')} disabled={remove.isPending} onConfirm={() => remove.mutate(n.id ?? '')} />
            </li>
          ))}
        </ul>
      )}
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <TextArea label={t('contacts.newNote')} maxLength={10000} value={text} errors={errors.field('note')} onChange={(e) => setText(e.target.value)} />
        <CheckboxInput label={t('contacts.isTodo')} checked={todo} onChange={(e) => setTodo(e.target.checked)} />
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={add.isPending || !text.trim()}>{t('contacts.addNote')}</button>
        </div>
      </form>
    </DetailBlock>
  )
}
