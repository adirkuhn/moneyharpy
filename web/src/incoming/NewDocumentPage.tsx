import { type DragEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { ListIcon, type ListIconKind } from '../layout/icons'
import { Page } from '../layout/Page'
import { documentPath, type EmptyType, useCreateEmpty, useUploadTogether } from './documents'

const maxSize = 10 * 1024 * 1024

/** The types to start without a document, in Moneybird's order. */
const types: { key: EmptyType | 'general_journal_document'; icon: ListIconKind }[] = [
  { key: 'purchase_invoice', icon: 'file' },
  { key: 'receipt', icon: 'coins' },
  { key: 'general_document', icon: 'paperclip' },
  { key: 'general_journal_document', icon: 'scale' },
]

/** The first step of adding an incoming document (/documents/new): its files, or else its type. */
export function NewDocumentPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const navigate = useNavigate()
  const upload = useUploadTogether(administrationId)
  const create = useCreateEmpty(administrationId)
  const [over, setOver] = useState(false)
  const [problems, setProblems] = useState<string[]>([])
  const [withoutDocument, setWithoutDocument] = useState(false)
  const typeId = useId()
  const base = `/${administrationId}/documents`
  const busy = upload.isPending || create.isPending

  const take = (list: FileList | null) => {
    const files = [...(list ?? [])]
    const tooLarge = files.filter((f) => f.size > maxSize)
    const fitting = files.filter((f) => f.size <= maxSize)
    setProblems(tooLarge.length > 0 ? [t('incoming.tooLarge', { names: tooLarge.map((f) => f.name).join(', ') })] : [])
    if (fitting.length === 0) {
      return
    }
    upload.mutate(fitting, {
      onSuccess: ({ id, refused }) => {
        if (id) {
          navigate(`${base}/${id}`)
        } else {
          setProblems((p) => [...p, t('incoming.refused', { names: refused.join(', ') })])
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
    <Page wide title={t('incoming.new.title')} back={{ to: base, label: t('incoming.new.back') }}>
      <div className="add-document">
        <div className="add-document__files">
          <label
            className={`add-document__drop${over ? ' is-over' : ''}`}
            onDragOver={(e) => { e.preventDefault(); setOver(true) }}
            onDragLeave={() => setOver(false)}
            onDrop={drop}
          >
            <span className="add-document__icon" aria-hidden="true"><ListIcon kind="upload" /></span>
            <span className="add-document__title">{upload.isPending ? t('incoming.uploading') : t('incoming.new.dropTitle')}</span>
            <span className="add-document__subtitle">{t('incoming.new.dropSubtitle')}</span>
            <input
              className="sr-only"
              type="file"
              multiple
              accept="application/pdf,image/*,text/plain"
              aria-label={t('incoming.new.dropTitle')}
              disabled={busy}
              onChange={(e) => {
                take(e.target.files)
                e.target.value = ''
              }}
            />
          </label>
          <p className="add-document__note">
            {t('incoming.new.note')} <Link to={base}>{t('incoming.new.noteLink')}</Link> {t('incoming.new.notePage')}
          </p>
          <Problems messages={[...problems, ...formErrors(upload.error, {}).general]} />
        </div>
        <div className="add-document__side">
          {withoutDocument ? (
            <section className="add-document__types" aria-labelledby={typeId}>
              <h2 id={typeId} className="add-document__label">{t('incoming.new.type')}</h2>
              <ul className="link-list" aria-labelledby={typeId}>
                {types.map(({ key, icon }) => {
                  const content = (
                    <>
                      <span className="link-list__icon" aria-hidden="true"><ListIcon kind={icon} /></span>
                      <span className="link-list__body">
                        <span className="link-list__title">{t(`incoming.types.${key}`)}</span>
                        <span className="link-list__subtitle">{t(`incoming.new.typeText.${key}`)}</span>
                      </span>
                      <span className="link-list__chevron" aria-hidden="true">›</span>
                    </>
                  )
                  return (
                    <li key={key}>
                      {key === 'general_journal_document' ? (
                        <Link className="link-list__item" to={`${base}/general_journal_documents/new`}>{content}</Link>
                      ) : (
                        <button
                          type="button"
                          className="link-list__item"
                          disabled={busy}
                          onClick={() => create.mutate(key, { onSuccess: (made) => navigate(documentPath(administrationId, made)) })}
                        >
                          {content}
                        </button>
                      )}
                    </li>
                  )
                })}
              </ul>
              <Problems messages={formErrors(create.error, {}).general} />
            </section>
          ) : (
            <button type="button" className="btn btn--secondary" onClick={() => setWithoutDocument(true)}>{t('incoming.new.withoutDocument')}</button>
          )}
        </div>
      </div>
    </Page>
  )
}
