import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { attachmentURL } from '../invoices/queries'
import { ListIcon } from '../layout/icons'
import type { ExternalInvoice } from './queries'

/**
 * The files of an external invoice under its document, as Moneybird's
 * preview list. Files are served as downloads, so they are listed rather
 * than shown; they are added on the edit page.
 */
export function AttachmentPreviews({ administrationId, invoice }: { administrationId: string; invoice: ExternalInvoice }) {
  const { t } = useTranslation()
  const id = useId()
  const files = invoice.attachments ?? []
  if (files.length === 0) {
    return null
  }
  return (
    <section className="attachment-previews" aria-labelledby={id}>
      <h2 id={id} className="sr-only">{t('invoices.view.attachments.title')}</h2>
      {files.map((a) => {
        const url = attachmentURL(administrationId, invoice.id ?? '', a.id ?? '', 'external')
        return (
          <figure key={a.id} className="attachment-previews__file">
            <figcaption className="attachment-previews__caption">
              <ListIcon kind="paperclip" />
              <a href={url} download={a.filename}>{a.filename}</a>
            </figcaption>
          </figure>
        )
      })}
    </section>
  )
}
