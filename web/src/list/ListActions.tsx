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
  /** Actions of this list only, placed after the export menu, such as Moneybird's Import. */
  trailing?: ReactNode
  primary: { to: string; label: string }
}

/** An entry of the ⋯ menu: a page, a file to download or open, or an action on this page. */
export type MenuItem =
  | { label: string; to: string }
  | { label: string; href: string; download?: boolean }
  | { label: string; onSelect: () => void; disabled?: boolean; danger?: boolean }

/**
 * The actions on the right of a list's action bar, after Moneybird: information,
 * statistics, export and the primary action. Lists have no ⋯ menu there.
 */
export function ListActions({ info, exports, extra, trailing, primary }: Props) {
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
      {trailing}
      <Link className="btn btn--accent btn--with-icon" to={primary.to}>
        <ListIcon kind="plus" />
        <span>{primary.label}</span>
      </Link>
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

/** The ⋯ button of an action bar, after Moneybird: the actions that do not need a button of their own. */
export function MoreMenu({ items }: { items: MenuItem[] }) {
  const { t } = useTranslation()
  const id = useId()
  const { open, toggle, close, root, button } = useDisclosure()
  const entry = (item: MenuItem) => {
    if ('to' in item) {
      return <Link className="dropdown__item" to={item.to} onClick={close}>{item.label}</Link>
    }
    if ('href' in item) {
      return item.download
        ? <a className="dropdown__item" href={item.href} download onClick={close}>{item.label}</a>
        : <a className="dropdown__item" href={item.href} target="_blank" rel="noreferrer" onClick={close}>{item.label}</a>
    }
    return (
      <button
        type="button"
        className={`dropdown__item dropdown__action${item.danger ? ' is-danger' : ''}`}
        disabled={item.disabled}
        onClick={() => {
          close()
          item.onSelect()
        }}
      >
        {item.label}
      </button>
    )
  }
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
            {items.map((item) => <li key={item.label}>{entry(item)}</li>)}
          </ul>
        </div>
      )}
    </div>
  )
}
