import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import type { components } from '../api/schema'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { DangerDialog } from '../layout/Modal'
import { PageIcon } from '../layout/pageIcons'
import { attachmentURL, type DocumentType, useAddAttachment, useDeleteAttachment } from './queries'

type Attachment = components['schemas']['attachment_response']

type Props = {
  administrationId: string
  type: DocumentType
  documentId: string
  attachments: Attachment[]
}

/** Images and PDFs show in the page; other files only download. */
const previewKind = (contentType: string) => (contentType.startsWith('image/') ? 'image' : contentType === 'application/pdf' ? 'pdf' : null)

/** The label of a file's tile, such as PDF or JPG, from its name or else its type. */
export function fileBadge(a: Pick<Attachment, 'filename' | 'content_type'>) {
  const extension = /\.([a-z0-9]{1,5})$/i.exec(a.filename ?? '')?.[1]
  return (extension ?? a.content_type?.split('/')[1] ?? '').toUpperCase()
}

const withoutExtension = (name: string) => name.replace(/\.[a-z0-9]{1,5}$/i, '')

/**
 * The file, fetched with the session and kept as an object URL in its own
 * type: the download answers as a file to save, which the browser would not
 * show in the page.
 */
function usePreview(url: string, contentType: string, enabled: boolean) {
  const file = useQuery({
    queryKey: ['attachment-preview', url],
    enabled,
    staleTime: Infinity,
    queryFn: async () => {
      const response = await fetch(new URL(url, window.location.origin))
      if (!response.ok) {
        throw new Error(String(response.status))
      }
      return new Blob([await response.blob()], { type: contentType })
    },
  })
  const [src, setSrc] = useState<string | null>(null)
  useEffect(() => {
    if (!file.data || typeof URL.createObjectURL !== 'function') {
      return
    }
    const made = URL.createObjectURL(file.data)
    setSrc(made)
    return () => {
      if (typeof URL.revokeObjectURL === 'function') {
        URL.revokeObjectURL(made)
      }
      setSrc(null)
    }
  }, [file.data])
  return src
}

/**
 * Moneybird's attachment viewer at the left of a document: the file shown
 * large, its name with download and delete, and a tile per file with an Add
 * tile to upload more.
 */
export function AttachmentViewer({ administrationId, type, documentId, attachments }: Props) {
  const { t } = useTranslation()
  const [chosenId, setChosenId] = useState<string | undefined>(undefined)
  const [deleting, setDeleting] = useState(false)
  const add = useAddAttachment(administrationId, type, documentId)
  const remove = useDeleteAttachment(administrationId, type, documentId)
  const shown = attachments.find((a) => a.id === chosenId) ?? attachments[0]
  const url = shown ? attachmentURL(administrationId, type, documentId, shown.id ?? '') : ''
  const kind = shown ? previewKind(shown.content_type ?? '') : null
  const src = usePreview(url, shown?.content_type ?? '', !!shown && kind !== null)

  return (
    <section className="attachment-viewer" aria-label={t('incoming.view.attachments')}>
      <div className="attachment-viewer__stage">
        {!shown && <p className="attachment-viewer__empty">{t('incoming.view.noAttachment')}</p>}
        {shown && kind === null && <p className="attachment-viewer__empty">{t('incoming.view.noPreview')}</p>}
        {shown && src && kind === 'image' && <img className="attachment-viewer__image" src={src} alt={shown.filename} />}
        {shown && src && kind === 'pdf' && <iframe className="attachment-viewer__pdf" src={src} title={shown.filename} />}
      </div>
      <div className="attachment-viewer__bar">
        {shown && (
          <div className="attachment-viewer__header">
            <p className="attachment-viewer__name">{shown.filename}</p>
            <div className="attachment-viewer__actions">
              <a className="btn btn--inline btn--icon" href={url} download={shown.filename} aria-label={t('incoming.view.download')} title={t('incoming.view.download')}>
                <PageIcon kind="download" />
              </a>
              <button type="button" className="btn btn--inline btn--icon" aria-label={t('incoming.view.deleteAttachment')} title={t('incoming.view.deleteAttachment')} onClick={() => setDeleting(true)}>
                <PageIcon kind="trash" />
              </button>
            </div>
          </div>
        )}
        <div className="attachment-viewer__row">
          {attachments.map((a) => (
            <button
              key={a.id}
              type="button"
              className={`file-tile${a.id === shown?.id ? ' is-active' : ''}`}
              aria-label={a.filename}
              aria-pressed={a.id === shown?.id}
              onClick={() => setChosenId(a.id)}
            >
              <span className="file-tile__thumb"><span className="file-tile__badge">{fileBadge(a)}</span></span>
              <span className="file-tile__name">{withoutExtension(a.filename ?? '')}</span>
            </button>
          ))}
          <label className={`file-tile file-tile--add${add.isPending ? ' is-busy' : ''}`}>
            <ListIcon kind="upload" />
            <span>{add.isPending ? t('incoming.uploading') : t('incoming.view.add')}</span>
            <input
              className="sr-only"
              type="file"
              multiple
              accept="application/pdf,image/*,text/plain"
              disabled={add.isPending}
              onChange={(e) => {
                for (const file of e.target.files ?? []) {
                  add.mutate(file)
                }
                e.target.value = ''
              }}
            />
          </label>
        </div>
        <Problems messages={[...formErrors(add.error, {}).general, ...formErrors(remove.error, {}).general]} />
      </div>
      {deleting && shown && (
        <DangerDialog
          question={t('incoming.view.confirmDeleteAttachment')}
          busy={remove.isPending}
          onCancel={() => setDeleting(false)}
          onConfirm={() => remove.mutate(shown.id ?? '', { onSuccess: () => setDeleting(false) })}
        />
      )}
    </section>
  )
}
