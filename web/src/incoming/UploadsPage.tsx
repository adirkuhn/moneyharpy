import { type DragEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { perPage } from '../invoices/queries'
import { ListIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { DocumentTable, listDate } from '../list/DocumentTable'
import { incomingAttachmentURL, useDeleteTypeless, useTypelessDocuments, useUploadDocuments } from './queries'

const maxSize = 10 * 1024 * 1024

/** The uploads still to process into a purchase invoice, receipt or other document (/documents/typeless_documents). */
export function UploadsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const uploads = useTypelessDocuments(administrationId, page)
  const remove = useDeleteTypeless(administrationId)
  const shown = uploads.data?.documents ?? []

  return (
    <Page wide title={t('purchases.titles.uploads')}>
      <UploadArea administrationId={administrationId} />
      <Problems messages={formErrors(remove.error, {}).general} />
      {!uploads.data ? (
        uploads.error ? <p className="empty">{t('purchases.loadError')}</p> : <Loading />
      ) : shown.length === 0 ? (
        <p className="empty">{t('purchases.noUploads')}</p>
      ) : (
        <DocumentTable
          label={t('purchases.titles.uploads')}
          rows={shown}
          rowKey={(d) => d.id ?? ''}
          href={() => undefined}
          total={uploads.data.total}
          perPage={perPage}
          columns={[
            {
              key: 'file', header: t('purchases.file'),
              cell: (d) => {
                const file = d.attachments?.[0]
                return file
                  ? <a href={incomingAttachmentURL(administrationId, 'typeless_documents', d.id ?? '', file.id ?? '')} target="_blank" rel="noreferrer">{file.filename}</a>
                  : d.reference
              },
            },
            { key: 'date', header: t('purchases.date'), kind: 'date', cell: (d) => listDate(d.date) },
            {
              key: 'actions', header: '', label: t('purchases.delete'),
              cell: (d) => (
                <button type="button" className="btn btn--secondary btn--small" disabled={remove.isPending} onClick={() => remove.mutate(d.id ?? '')}>
                  {t('purchases.delete')}
                </button>
              ),
            },
          ]}
        />
      )}
    </Page>
  )
}

/** Takes files, dropped or chosen, and keeps each as an upload to process. */
function UploadArea({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const id = useId()
  const upload = useUploadDocuments(administrationId)
  const [over, setOver] = useState(false)
  const [problems, setProblems] = useState<string[]>([])

  const take = (list: FileList | null) => {
    const files = [...(list ?? [])]
    const tooLarge = files.filter((f) => f.size > maxSize)
    const fitting = files.filter((f) => f.size <= maxSize)
    setProblems(tooLarge.length > 0 ? [t('purchases.tooLarge', { names: tooLarge.map((f) => f.name).join(', ') })] : [])
    if (fitting.length === 0) {
      return
    }
    upload.mutate(fitting, {
      onSuccess: ({ refused }) => {
        if (refused.length > 0) {
          setProblems((p) => [...p, t('purchases.refused', { names: refused.join(', ') })])
        }
      },
    })
  }
  const drop = (e: DragEvent) => {
    e.preventDefault()
    setOver(false)
    take(e.dataTransfer.files)
  }

  return (
    <section
      className={`upload-area${over ? ' is-over' : ''}`}
      aria-labelledby={`${id}-title`}
      onDragOver={(e) => { e.preventDefault(); setOver(true) }}
      onDragLeave={() => setOver(false)}
      onDrop={drop}
    >
      <h2 id={`${id}-title`} className="sr-only">{t('purchases.uploadTitle')}</h2>
      <p className="upload-area__text">
        <ListIcon kind="paperclip" />
        <span>
          {t('purchases.dropText')}{' '}
          <label className="upload-area__pick" htmlFor={`${id}-file`}>{t('purchases.select')}</label>
        </span>
      </p>
      <p className="hint">{upload.isPending ? t('invoices.uploading') : t('purchases.uploadHelp')}</p>
      <Problems messages={[...problems, ...formErrors(upload.error, {}).general]} />
      <input
        id={`${id}-file`}
        className="sr-only"
        type="file"
        multiple
        accept="application/pdf,image/*,text/plain"
        aria-label={t('purchases.upload')}
        disabled={upload.isPending}
        onChange={(e) => {
          take(e.target.files)
          e.target.value = ''
        }}
      />
    </section>
  )
}
