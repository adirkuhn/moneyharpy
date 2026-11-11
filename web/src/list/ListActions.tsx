import { type ReactNode, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { CaretIcon, InfoIcon, ListIcon, type ListIconKind } from '../layout/icons'
import { useDisclosure } from './useDisclosure'

type Props = {
  /** The button that shows or hides the list's introduction. */
  info?: { shown: boolean; onToggle: () => void; controls: string }
  /** What the export menu will offer; none are available yet. */
  exports: string[]
  /** Actions of this list only, placed before the export menu. */
  extra?: ReactNode
  primary: { to: string; label: string }
  /** Links under the ⋯ button. */
  more: { to: string; label: string }[]
}

/** The actions on the right of a list's action bar, after Moneybird: information, statistics, export, the primary action and more. */
export function ListActions({ info, exports, extra, primary, more }: Props) {
  const { t } = useTranslation()
  return (
    <>
      {info && (
        <button
          type="button"
          className={`btn btn--secondary btn--icon${info.shown ? ' is-active' : ''}`}
          aria-label={t('list.moreInfo')}
          title={t('list.moreInfo')}
          aria-expanded={info.shown}
          aria-controls={info.controls}
          onClick={info.onToggle}
        >
          <InfoIcon />
        </button>
      )}
      <Unavailable icon="chart" label={t('list.statistics')} />
      {extra}
      {exports.length > 0 ? <ExportMenu entries={exports} /> : <Unavailable icon="export" label={t('list.export')} text />}
      <Link className="btn btn--accent btn--with-icon" to={primary.to}>
        <ListIcon kind="plus" />
        <span>{primary.label}</span>
      </Link>
      {more.length > 0 && <MoreMenu links={more} />}
    </>
  )
}

/** A button for something MoneyHarpy does not do yet: it can be focused and says so. */
export function Unavailable({ icon, label, text = false }: { icon: ListIconKind; label: string; text?: boolean }) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <span className="unavailable">
      <button
        type="button"
        className={`btn btn--secondary ${text ? 'btn--with-icon' : 'btn--icon'}`}
        aria-disabled="true"
        aria-label={text ? undefined : label}
        aria-describedby={id}
      >
        <ListIcon kind={icon} />
        {text && <span>{label}</span>}
      </button>
      <span id={id} role="tooltip" className="unavailable__tip">{t('list.notAvailable')}</span>
    </span>
  )
}

function ExportMenu({ entries }: { entries: string[] }) {
  const { t } = useTranslation()
  const id = useId()
  const { open, toggle, root, button } = useDisclosure()
  return (
    <div className="dropdown" ref={root}>
      <button ref={button} type="button" className="btn btn--secondary btn--with-icon" aria-expanded={open} aria-controls={id} onClick={toggle}>
        <ListIcon kind="export" />
        <span>{t('list.export')}</span>
        <CaretIcon />
      </button>
      {open && (
        <div id={id} className="dropdown__panel dropdown__panel--right">
          <p className="dropdown__header">{t('list.export')}</p>
          <ul className="dropdown__list">
            {entries.map((entry) => (
              <li key={entry}>
                <span className="dropdown__item is-disabled" aria-disabled="true">
                  <span>{entry}</span>
                  <span className="dropdown__hint">{t('list.notAvailable')}</span>
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function MoreMenu({ links }: { links: { to: string; label: string }[] }) {
  const { t } = useTranslation()
  const id = useId()
  const { open, toggle, root, button } = useDisclosure()
  return (
    <div className="dropdown" ref={root}>
      <button
        ref={button}
        type="button"
        className="btn btn--secondary btn--icon"
        aria-label={t('list.moreActions')}
        title={t('list.moreActions')}
        aria-expanded={open}
        aria-controls={id}
        onClick={toggle}
      >
        <ListIcon kind="more" />
      </button>
      {open && (
        <div id={id} className="dropdown__panel dropdown__panel--right">
          <ul className="dropdown__list">
            {links.map((link) => (
              <li key={link.to}>
                <Link className="dropdown__item" to={link.to}>{link.label}</Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
