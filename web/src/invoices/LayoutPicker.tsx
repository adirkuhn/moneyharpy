import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { type DocumentStyle, styleFileURL } from '../invoicing/queries'

type Props = {
  id: string
  administrationId: string
  styles: DocumentStyle[]
  /** The style the invoice uses now. */
  current: string
  onPick: (id: string) => void
}

/** Moneybird's "Select layout" strip under the action bar: each document style with a small preview. */
export function LayoutPicker({ id, administrationId, styles, current, onPick }: Props) {
  const { t } = useTranslation()
  const heading = useId()
  return (
    <section id={id} className="layout-picker" aria-labelledby={heading}>
      <h3 id={heading} className="layout-picker__title">{t('invoices.editor.selectLayout')}</h3>
      <ul className="layout-picker__list">
        {styles.map((s) => (
          <li key={s.id}>
            <button type="button" className="layout-picker__style" aria-pressed={s.id === current} onClick={() => onPick(s.id ?? '')}>
              <StylePreview administrationId={administrationId} style={s} />
              <span className="layout-picker__name">{s.name}</span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

/** A small drawing of a document style: its logo, sender, recipient, title and lines where the style puts them. */
export function StylePreview({ administrationId, style: s }: { administrationId: string; style: DocumentStyle }) {
  return (
    <span className={`layout-picker__preview layout-picker__preview--${s.address_position ?? 'left'}`} aria-hidden="true">
      {s.logo_hash
        ? <img className={`layout-picker__logo layout-picker__logo--${s.logo_position ?? 'left'}`} src={styleFileURL(administrationId, s.id ?? '', 'logo', s.logo_hash)} alt="" />
        : <span className={`layout-picker__logo layout-picker__logo--${s.logo_position ?? 'left'}`} />}
      <span className="layout-picker__sender" />
      <span className="layout-picker__recipient" />
      <span className="layout-picker__title-bar" />
      <span className="layout-picker__rows" />
    </span>
  )
}
