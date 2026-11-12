import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { DetailBlock } from '../detail/Detail'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, formErrors, TextArea } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import {
  attachmentURL, type Collectable, type InvoiceKind, useAddInvoiceAttachment, useAddInvoiceNote, useDeleteInvoiceAttachment, useDeleteInvoiceNote,
} from './queries'

type Props = { administrationId: string; invoice: Collectable; kind?: InvoiceKind }

/** Notes and to-dos kept with an invoice. */
export function InvoiceNotes({ administrationId, invoice, kind = 'sales' }: Props) {
  const { t, i18n } = useTranslation()
  const [text, setText] = useState('')
  const [todo, setTodo] = useState(false)
  const add = useAddInvoiceNote(administrationId, invoice.id ?? '', kind)
  const remove = useDeleteInvoiceNote(administrationId, invoice.id ?? '', kind)
  const errors = formErrors(add.error ?? remove.error, { note: t('contacts.newNote') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    add.mutate({ note: text, todo }, { onSuccess: () => { setText(''); setTodo(false) } })
  }
  const notes = invoice.notes ?? []
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

const maxSize = 10 * 1024 * 1024

/**
 * The files of a sales invoice under its document, after Moneybird: the
 * files added so far, and a zone to drop more on or pick them, at most 10 MB
 * in all.
 */
export function AttachmentDropZone({ administrationId, invoice, kind = 'sales' }: Props) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const [problem, setProblem] = useState<string | null>(null)
  const [over, setOver] = useState(false)
  const add = useAddInvoiceAttachment(administrationId, invoice.id ?? '', kind)
  const remove = useDeleteInvoiceAttachment(administrationId, invoice.id ?? '', kind)
  const attachments = invoice.attachments ?? []
  const problems = [...(problem ? [problem] : []), ...formErrors(add.error ?? remove.error, {}).general]

  const upload = async (files: File[]) => {
    if (files.length === 0) {
      return
    }
    if (files.some((f) => f.size > maxSize)) {
      setProblem(t('invoices.view.attachments.tooLarge'))
      return
    }
    const used = attachments.reduce((sum, a) => sum + (a.size ?? 0), 0)
    if (used + files.reduce((sum, f) => sum + f.size, 0) > maxSize) {
      setProblem(t('invoices.view.attachments.totalTooLarge'))
      return
    }
    setProblem(null)
    for (const file of files) {
      try {
        await add.mutateAsync(file)
      } catch {
        // The error is shown from the mutation; the other files are not sent.
        return
      }
    }
  }

  return (
    <section className="attachment-zone" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="sr-only">{t('invoices.view.attachments.title')}</h2>
      <Problems messages={problems} />
      {attachments.length > 0 && (
        <ul className="attachment-zone__files">
          {attachments.map((a) => (
            <li key={a.id} className="attachment-zone__file">
              <ListIcon kind="paperclip" />
              <a href={attachmentURL(administrationId, invoice.id ?? '', a.id ?? '', kind)} download={a.filename}>{a.filename}</a>
              <span className="hint">{size(a.size ?? 0, i18n.language)}</span>
              <ConfirmDelete
                label={t('invoices.removeAttachment', { name: a.filename })}
                confirmLabel={t('invoices.confirmRemoveAttachment', { name: a.filename })}
                text={t('contacts.remove')}
                confirmText={t('contacts.confirmRemove')}
                disabled={remove.isPending}
                onConfirm={() => remove.mutate(a.id ?? '')}
              />
            </li>
          ))}
        </ul>
      )}
      <div
        className={`attachment-zone__drop${over ? ' is-over' : ''}`}
        onDragOver={(e) => {
          e.preventDefault()
          setOver(true)
        }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => {
          e.preventDefault()
          setOver(false)
          void upload(Array.from(e.dataTransfer.files))
        }}
      >
        <ListIcon kind="upload" />
        <span>
          {t('invoices.view.attachments.drag')}{' '}
          <label className="attachment-zone__pick" htmlFor={`${id}-file`}>{t('invoices.view.attachments.select')}</label>{' '}
          {t('invoices.view.attachments.max')}
        </span>
        <input
          id={`${id}-file`}
          className="sr-only"
          type="file"
          multiple
          disabled={add.isPending}
          onChange={(e) => {
            const files = Array.from(e.target.files ?? [])
            e.target.value = ''
            void upload(files)
          }}
        />
        {add.isPending && <span className="hint" role="status">{t('invoices.uploading')}</span>}
      </div>
    </section>
  )
}

function size(bytes: number, language: string) {
  const kb = bytes / 1024
  const unit = kb >= 1024 ? 'megabyte' : 'kilobyte'
  return new Intl.NumberFormat(language, { style: 'unit', unit, maximumFractionDigits: 1 }).format(kb >= 1024 ? kb / 1024 : Math.max(kb, 0.1))
}

/** Files that go with an invoice, such as an order or a timesheet. */
export function InvoiceAttachments({ administrationId, invoice, kind = 'sales' }: Props) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const [tooLarge, setTooLarge] = useState(false)
  const add = useAddInvoiceAttachment(administrationId, invoice.id ?? '', kind)
  const remove = useDeleteInvoiceAttachment(administrationId, invoice.id ?? '', kind)
  const problems = [...(tooLarge ? [t('invoices.attachmentTooLarge')] : []), ...formErrors(add.error ?? remove.error, {}).general]
  const attachments = invoice.attachments ?? []
  return (
    <DetailBlock title={t('invoices.attachments')}>
      <Problems messages={problems} />
      {attachments.length === 0 ? (
        <p className="empty">{t('invoices.noAttachments')}</p>
      ) : (
        <ul className="notes">
          {attachments.map((a) => (
            <li key={a.id} className="notes__row">
              <div>
                <a href={attachmentURL(administrationId, invoice.id ?? '', a.id ?? '', kind)} download={a.filename}>{a.filename}</a>
                <p className="hint">{size(a.size ?? 0, i18n.language)}</p>
              </div>
              <ConfirmDelete
                label={t('invoices.removeAttachment', { name: a.filename })}
                confirmLabel={t('invoices.confirmRemoveAttachment', { name: a.filename })}
                text={t('contacts.remove')}
                confirmText={t('contacts.confirmRemove')}
                disabled={remove.isPending}
                onConfirm={() => remove.mutate(a.id ?? '')}
              />
            </li>
          ))}
        </ul>
      )}
      <div className="file-field">
        <label className="btn btn--secondary btn--small file-field__pick" htmlFor={`${id}-file`}>{t('invoices.addAttachment')}</label>
        <input
          id={`${id}-file`}
          className="sr-only"
          type="file"
          accept="application/pdf,image/*,text/plain"
          aria-describedby={`${id}-help`}
          disabled={add.isPending}
          onChange={(e) => {
            const file = e.target.files?.[0]
            e.target.value = ''
            if (!file) {
              return
            }
            setTooLarge(file.size > maxSize)
            if (file.size <= maxSize) {
              add.mutate(file)
            }
          }}
        />
        <span id={`${id}-help`} className="hint">{add.isPending ? t('invoices.uploading') : t('invoices.attachmentHelp')}</span>
      </div>
    </DetailBlock>
  )
}
