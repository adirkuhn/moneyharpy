import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import type { components } from '../api/schema'
import { formErrors } from '../forms/Form'
import { Remark } from '../forms/Remark'
import { ListIcon } from '../layout/icons'
import { DangerDialog } from '../layout/Modal'
import { PageIcon } from '../layout/pageIcons'
import { attachmentURL, type DocumentType, previewURL, useAddAttachment, useDeleteAttachment } from './queries'

type Attachment = components['schemas']['attachment_response']

type Props = {
  administrationId: string
  type: DocumentType
  documentId: string
  attachments: Attachment[]
  /** The attachment Scan and Recognise read the document from, marked with a bolt. */
  analysedId?: string
}

/** Images and PDFs show in the page; other files only download. */
const previewKind = (contentType: string) => (contentType.startsWith('image/') ? 'image' : contentType === 'application/pdf' ? 'pdf' : null)

/** The label of a file's tile, such as PDF or JPG, from its name or else its type. */
export function fileBadge(a: Pick<Attachment, 'filename' | 'content_type'>) {
  const extension = /\.([a-z0-9]{1,5})$/i.exec(a.filename ?? '')?.[1]
  return (extension ?? a.content_type?.split('/')[1] ?? '').toUpperCase()
}

/** The colour of a file's badge: PDF green, email red, images blue, others grey. */
export function badgeColour(a: Pick<Attachment, 'filename' | 'content_type'>) {
  const badge = fileBadge(a)
  const type = a.content_type ?? ''
  if (badge === 'PDF' || type === 'application/pdf') {
    return 'green'
  }
  if (badge === 'EML' || badge === 'MSG' || type === 'message/rfc822') {
    return 'red'
  }
  return type.startsWith('image/') ? 'blue' : 'grey'
}

const withoutExtension = (name: string) => name.replace(/\.[a-z0-9]{1,5}$/i, '')

/**
 * The first page of the server's preview of an attachment, and how many pages
 * there are. Without the OCR service there is none (404), and the page shows
 * the file itself.
 */
function usePagePreview(url: string, enabled: boolean) {
  return useQuery({
    queryKey: ['attachment-pages', url],
    enabled,
    staleTime: Infinity,
    queryFn: async () => {
      const response = await fetch(new URL(url, window.location.origin))
      if (!response.ok) {
        return null
      }
      return { first: await response.blob(), count: Math.max(1, Number(response.headers.get('X-Page-Count')) || 1) }
    },
  })
}

/** An object URL of a file kept while it is shown. */
function useObjectURL(file: Blob | undefined) {
  const [src, setSrc] = useState<string | null>(null)
  useEffect(() => {
    if (!file || typeof URL.createObjectURL !== 'function') {
      return
    }
    const made = URL.createObjectURL(file)
    setSrc(made)
    return () => {
      if (typeof URL.revokeObjectURL === 'function') {
        URL.revokeObjectURL(made)
      }
      setSrc(null)
    }
  }, [file])
  return src
}

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
  return useObjectURL(file.data)
}

type PagesProps = { name: string; first: Blob; count: number; pageURL: (page: number) => string }

/**
 * Moneybird's preview: the pages of the file as images, one under the other
 * on the grey stage, the first at once and the others as they scroll into
 * view. A click zooms in, and another out.
 */
function Pages({ name, first, count, pageURL }: PagesProps) {
  const { t } = useTranslation()
  const [zoomed, setZoomed] = useState(false)
  const src = useObjectURL(first)
  const alt = (page: number) => t('incoming.view.page', { name, page })
  return (
    <button
      type="button"
      className={`attachment-viewer__preview${zoomed ? ' attachment-viewer__preview--zoomed' : ''}`}
      aria-label={t('incoming.view.zoomIn')}
      aria-pressed={zoomed}
      title={t(zoomed ? 'incoming.view.zoomOut' : 'incoming.view.zoomIn')}
      onClick={() => setZoomed(!zoomed)}
    >
      {src && <img className="attachment-viewer__image" src={src} alt={alt(1)} draggable={false} />}
      {Array.from({ length: count - 1 }, (_, i) => i + 2).map((page) => (
        <img key={page} className="attachment-viewer__image" src={pageURL(page)} alt={alt(page)} loading="lazy" draggable={false} />
      ))}
    </button>
  )
}

/**
 * Moneybird's attachment viewer at the left of a document: the file shown
 * large, its name with download and delete, and a tile per file with an Add
 * tile to upload more.
 */
export function AttachmentViewer({ administrationId, type, documentId, attachments, analysedId }: Props) {
  const { t } = useTranslation()
  const [chosenId, setChosenId] = useState<string | undefined>(undefined)
  const [deleting, setDeleting] = useState(false)
  const add = useAddAttachment(administrationId, type, documentId)
  const remove = useDeleteAttachment(administrationId, type, documentId)
  const shown = attachments.find((a) => a.id === chosenId) ?? attachments[0]
  const url = shown ? attachmentURL(administrationId, type, documentId, shown.id ?? '') : ''
  const kind = shown ? previewKind(shown.content_type ?? '') : null
  const pageURL = (page: number) => previewURL(administrationId, documentId, shown?.id ?? '', page)
  const pages = usePagePreview(pageURL(1), !!shown && kind !== null)
  // The file itself, when the server has no preview of it.
  const src = usePreview(url, shown?.content_type ?? '', !!shown && kind !== null && (pages.data === null || pages.isError))

  return (
    <section className="attachment-viewer" aria-label={t('incoming.view.attachments')}>
      <div className="attachment-viewer__stage">
        {!shown && <p className="attachment-viewer__empty">{t('incoming.view.noAttachment')}</p>}
        {shown && kind === null && <p className="attachment-viewer__empty">{t('incoming.view.noPreview')}</p>}
        {shown && pages.data && <Pages key={shown.id} name={shown.filename ?? ''} first={pages.data.first} count={pages.data.count} pageURL={pageURL} />}
        {shown && src && kind === 'image' && <img className="attachment-viewer__image" src={src} alt={shown.filename} />}
        {shown && src && kind === 'pdf' && <iframe className="attachment-viewer__pdf" src={src} title={shown.filename} />}
      </div>
      <div className="attachment-viewer__bar">
        {shown && (
          <div className="attachment-viewer__header">
            <p className="attachment-viewer__name">{shown.filename}</p>
            <div className="attachment-viewer__actions">
              {shown.id === analysedId && <span className="attachment-viewer__marker"><Remark confidence="certain" label={t('incoming.view.analysed')} /></span>}
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
              <span className="file-tile__thumb"><span className={`file-tile__badge file-tile__badge--${badgeColour(a)}`}>{fileBadge(a)}</span></span>
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
