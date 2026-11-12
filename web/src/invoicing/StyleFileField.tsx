import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { type DocumentStyle, type FileKind, styleFileURL, useRemoveStyleFile, useUploadStyleFile } from './queries'

const accepted: Record<FileKind, string> = { logo: 'image/png,image/jpeg,image/gif', background: 'image/png,image/jpeg,application/pdf' }

type Props = { administrationId: string; style: DocumentStyle; kind: FileKind; onChange: (style: DocumentStyle) => void }

/**
 * A document style's logo or background, after Moneybird's image field: the
 * current file with buttons to open and delete it, above a drop zone.
 */
export function StyleFileField({ administrationId, style, kind, onChange }: Props) {
  const { t } = useTranslation()
  const id = useId()
  const upload = useUploadStyleFile(administrationId, style.id ?? '')
  const remove = useRemoveStyleFile(administrationId, style.id ?? '')
  const [dragging, setDragging] = useState(false)
  const what = t(`documentStyle.${kind}`)
  const hash = kind === 'logo' ? style.logo_hash : style.background_hash
  const url = hash ? styleFileURL(administrationId, style.id ?? '', kind, hash) : ''
  const errors = formErrors(upload.error ?? remove.error, { [kind]: what })
  const problems = [...(errors.field(kind) ?? []), ...errors.general]
  const send = (data: File | undefined) => data && upload.mutate({ file: kind, data }, { onSuccess: onChange })

  return (
    <div className="style-file">
      {hash && (
        <div className="style-file__current">
          {kind === 'logo' ? <img className="style-file__image" src={url} alt={t('documentStyle.preview', { what })} /> : <span className="style-file__name">{t('documentStyle.preview', { what })}</span>}
          <span className="style-file__actions">
            <a className="btn btn--secondary btn--small btn--icon" href={url} target="_blank" rel="noreferrer" aria-label={t('documentStyle.open', { what })} title={t('documentStyle.open', { what })}>
              <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" fill="none" stroke="currentColor" strokeWidth={1.75} aria-hidden="true"><path d="M2.500 12S6 5.500 12 5.500 21.500 12 21.500 12 18 18.500 12 18.500 2.500 12 2.500 12Z" /><circle cx="12" cy="12" r="3" /></svg>
            </a>
            <button type="button" className="btn btn--secondary btn--small btn--icon" aria-label={t('documentStyle.remove', { what })} title={t('documentStyle.remove', { what })} disabled={remove.isPending} onClick={() => remove.mutate(kind, { onSuccess: onChange })}>
              <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" aria-hidden="true"><path d="M4.500 7h15M9.500 7V4.500h5V7M6.500 7l1 13h9l1-13M10 11v5.500M14 11v5.500" /></svg>
            </button>
          </span>
        </div>
      )}
      <label
        className={`style-file__drop${dragging ? ' is-dragging' : ''}`}
        htmlFor={`${id}-file`}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault()
          setDragging(false)
          send(e.dataTransfer.files[0])
        }}
      >
        <ListIcon kind="upload" />
        <span>{upload.isPending ? t('documentStyle.uploading') : <>{t('documentStyle.drop')} <span className="style-file__select">{t('documentStyle.select')}</span></>}</span>
      </label>
      <input
        id={`${id}-file`}
        className="sr-only"
        type="file"
        accept={accepted[kind]}
        aria-label={t('documentStyle.upload', { what })}
        aria-describedby={`${id}-help`}
        onChange={(e) => {
          send(e.target.files?.[0])
          e.target.value = ''
        }}
      />
      <p id={`${id}-help`} className={`form-input__message form-input__message--${problems.length ? 'error' : 'help'}`}>
        {problems.length ? problems.join(' ') : t(`documentStyle.${kind}Help`)}
      </p>
    </div>
  )
}
