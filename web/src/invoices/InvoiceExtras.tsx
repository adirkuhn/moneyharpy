import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { attachmentURL, type Collectable, type InvoiceKind, useAddInvoiceAttachment, useDeleteInvoiceAttachment } from './queries'

type Props = { administrationId: string; invoice: Collectable; kind?: InvoiceKind }

const maxSize = 10 * 1024 * 1024
const externalTotal = 15 * 1024 * 1024

/**
 * The files of a sales invoice under its document, after Moneybird: the
 * files added so far, and a zone to drop more on or pick them, at most 10 MB
 * in all.
 */
export function AttachmentDropZone({ administrationId, invoice, kind = 'sales' }: Props) {
  // As at Moneybird, a sales invoice holds 10 MB of files and an external invoice 15 MB, each file at most 10 MB.
  const add = useAddInvoiceAttachment(administrationId, invoice.id ?? '', kind)
  const remove = useDeleteInvoiceAttachment(administrationId, invoice.id ?? '', kind)
  return (
    <AttachmentZone
      attachments={invoice.attachments ?? []}
      add={add}
      remove={remove}
      url={(a) => attachmentURL(administrationId, invoice.id ?? '', a, kind)}
      total={kind === 'external' ? externalTotal : maxSize}
      texts={kind === 'external' ? 'external.attachments' : 'invoices.view.attachments'}
    />
  )
}

type Attachment = NonNullable<Collectable['attachments']>[number]
type ZoneProps = {
  attachments: Attachment[]
  add: { mutateAsync: (file: File) => Promise<unknown>; isPending: boolean; error: unknown }
  remove: { mutate: (attachmentId: string) => void; isPending: boolean; error: unknown }
  /** Where an attachment is downloaded. */
  url: (attachmentId: string) => string
  /** The most all files together may weigh, in bytes. */
  total?: number
  /** Where the zone's texts about sizes are, such as invoices.view.attachments. */
  texts?: string
}

/** The files of a document under it and the zone to drop more on, for a document of any kind. */
export function AttachmentZone({ attachments, add, remove, url, total = maxSize, texts = 'invoices.view.attachments' }: ZoneProps) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const [problem, setProblem] = useState<string | null>(null)
  const [over, setOver] = useState(false)
  const problems = [...(problem ? [problem] : []), ...formErrors(add.error ?? remove.error, {}).general]

  const upload = async (files: File[]) => {
    if (files.length === 0) {
      return
    }
    if (files.some((f) => f.size > maxSize)) {
      setProblem(t(`${texts}.tooLarge`))
      return
    }
    const used = attachments.reduce((sum, a) => sum + (a.size ?? 0), 0)
    if (used + files.reduce((sum, f) => sum + f.size, 0) > total) {
      setProblem(t(`${texts}.totalTooLarge`))
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
              <a href={url(a.id ?? '')} download={a.filename}>{a.filename}</a>
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
          {t(`${texts}.max`)}
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
