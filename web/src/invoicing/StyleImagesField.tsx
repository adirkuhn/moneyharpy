import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { type DocumentStyle, useAddStyleImage, useRemoveStyleImage } from './queries'

type Props = { administrationId: string; style: DocumentStyle; onChange: (style: DocumentStyle) => void }

/**
 * The images custom CSS can use besides the logo and background, after
 * Moneybird's "CSS images": each with the address to use in url(), and a
 * drop zone to add more.
 */
export function StyleImagesField({ administrationId, style, onChange }: Props) {
  const { t } = useTranslation()
  const id = useId()
  const add = useAddStyleImage(administrationId, style.id ?? '')
  const remove = useRemoveStyleImage(administrationId, style.id ?? '')
  const errors = formErrors(add.error ?? remove.error, { image: t('documentStyle.cssImages') })
  const problems = [...(errors.field('image') ?? []), ...errors.general]
  const send = (files: FileList | null | undefined) => {
    for (const file of Array.from(files ?? [])) {
      add.mutate(file, { onSuccess: onChange })
    }
  }

  return (
    <div className="style-file">
      <p className="style-file__label" id={`${id}-label`}>{t('documentStyle.cssImages')}</p>
      {style.images.length > 0 && (
        <ul className="style-images" aria-labelledby={`${id}-label`}>
          {style.images.map((img, n) => (
            <li key={img.id} className="style-images__item">
              <img src={img.url} alt={t('documentStyle.cssImage', { n: n + 1 })} />
              <code className="style-images__url">url({img.url})</code>
              <button
                type="button"
                className="btn btn--secondary btn--small btn--icon"
                aria-label={t('documentStyle.removeImage', { n: n + 1 })}
                title={t('documentStyle.removeImage', { n: n + 1 })}
                disabled={remove.isPending}
                onClick={() => remove.mutate(img.id, { onSuccess: onChange })}
              >
                <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" aria-hidden="true"><path d="M4.500 7h15M9.500 7V4.500h5V7M6.500 7l1 13h9l1-13M10 11v5.500M14 11v5.500" /></svg>
              </button>
            </li>
          ))}
        </ul>
      )}
      <label
        className="style-file__drop"
        htmlFor={`${id}-file`}
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault()
          send(e.dataTransfer.files)
        }}
      >
        <ListIcon kind="upload" />
        <span>{add.isPending ? t('documentStyle.uploading') : <>{t('documentStyle.drop')} <span className="style-file__select">{t('documentStyle.select')}</span></>}</span>
      </label>
      <input
        id={`${id}-file`}
        className="sr-only"
        type="file"
        multiple
        accept="image/png,image/jpeg,image/gif"
        aria-label={t('documentStyle.uploadImages')}
        aria-describedby={`${id}-help`}
        onChange={(e) => {
          send(e.target.files)
          e.target.value = ''
        }}
      />
      <p id={`${id}-help`} className={`form-input__message form-input__message--${problems.length ? 'error' : 'help'}`}>
        {problems.length ? problems.join(' ') : t('documentStyle.cssImagesHelp')}
      </p>
    </div>
  )
}
