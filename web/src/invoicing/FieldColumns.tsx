import { type DragEvent, type KeyboardEvent, useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Line } from './queries'
import { hiddenFields, moveField, type Place } from './styleLayout'

type Props = {
  /** The titles of the shown columns, such as "Show", or "Left column" and "Right column". */
  titles: string[]
  columns: Line[][]
  /** The fields this block can show, in the order the hidden column lists them. */
  fields: readonly string[]
  /** Whether blank lines can be added, as in a sender address. */
  blankLines?: boolean
  /** What a field prints, such as the lines of the address; without it only its name shows. */
  value?: (field: string) => string[]
  /** Names of fields the translations do not know, such as custom fields. */
  names?: Record<string, string>
  onChange: (columns: Line[][]) => void
}

/**
 * Moneybird's drag-and-drop columns: the fields a block shows, in order, and
 * the ones it does not. Fields are dragged with the mouse or moved with the
 * arrow keys on their handle.
 */
export function FieldColumns({ titles, columns, fields, blankLines = false, value, names = {}, onChange }: Props) {
  const { t } = useTranslation()
  const helpId = useId()
  const hidden = hiddenFields(columns, fields, blankLines)
  const hiddenColumn = columns.length
  const handles = useRef(new Map<string, HTMLButtonElement>())
  const [focusAt, setFocusAt] = useState<Place | null>(null)
  const dragged = useRef<Place | null>(null)

  useEffect(() => {
    if (focusAt) {
      handles.current.get(`${focusAt.column}:${focusAt.index}`)?.focus()
      setFocusAt(null)
    }
  }, [focusAt])

  const move = (from: Place, to: Place, focus: boolean) => {
    const moved = moveField(columns, fields, blankLines, from, to)
    if (moved.columns !== columns) {
      onChange(moved.columns)
      if (focus) {
        setFocusAt(moved.at)
      }
    }
  }
  const setLine = (place: Place, change: Partial<Line>) =>
    onChange(columns.map((c, ci) => (ci === place.column ? c.map((l, li) => (li === place.index ? { ...l, ...change } : l)) : c)))

  const onKey = (place: Place, length: number) => (e: KeyboardEvent) => {
    const to: Record<string, Place | undefined> = {
      ArrowUp: place.column < hiddenColumn && place.index > 0 ? { column: place.column, index: place.index - 1 } : undefined,
      ArrowDown: place.column < hiddenColumn && place.index < length - 1 ? { column: place.column, index: place.index + 1 } : undefined,
      ArrowLeft: place.column > 0 ? { column: place.column - 1, index: Number.MAX_SAFE_INTEGER } : undefined,
      ArrowRight: place.column < hiddenColumn ? { column: place.column + 1, index: Number.MAX_SAFE_INTEGER } : undefined,
    }
    const target = to[e.key]
    if (e.key in to) {
      e.preventDefault()
    }
    if (target) {
      move(place, target, true)
    }
  }
  const drop = (to: Place) => (e: DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (dragged.current) {
      move(dragged.current, to, false)
      dragged.current = null
    }
  }

  const name = (field: string) => names[field] ?? t(`documentStyle.fields.${field}`)
  const item = (field: string, place: Place, length: number, line?: Line) => {
    const isBlank = field === 'newline'
    const lines = isBlank ? [] : value?.(field)
    return (
      <li
        key={`${place.column}:${place.index}:${field}`}
        className={`field-columns__item${line?.bold ? ' is-bold' : ''}${line?.italic ? ' is-italic' : ''}`}
        draggable
        onDragStart={(e) => {
          dragged.current = place
          e.dataTransfer.effectAllowed = 'move'
        }}
        onDragOver={(e) => e.preventDefault()}
        onDrop={drop(place)}
      >
        <button
          type="button"
          className="field-columns__handle"
          aria-label={t('documentStyle.move', { name: name(field) })}
          aria-describedby={helpId}
          ref={(el) => {
            const key = `${place.column}:${place.index}`
            if (el) {
              handles.current.set(key, el)
            } else {
              handles.current.delete(key)
            }
          }}
          onKeyDown={onKey(place, length)}
        >
          <svg viewBox="0 0 16 16" width="0.875em" height="0.875em" aria-hidden="true"><path d="M8 1.5v13M5 4.5l3-3 3 3M5 11.5l3 3 3-3" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg>
        </button>
        <span className="field-columns__body">
          {isBlank ? (
            <span className="field-columns__blank">{name(field)}</span>
          ) : value ? (
            <>
              <span className="field-columns__name">{name(field)}:</span>
              {lines && lines.length > 0 ? (
                lines.map((l, i) => <span key={i} className="field-columns__value">{l}</span>)
              ) : (
                <span className="field-columns__value is-empty" title={t('documentStyle.noValue')}>{name(field)}</span>
              )}
            </>
          ) : (
            <span className="field-columns__value">{name(field)}</span>
          )}
        </span>
        {line && !isBlank && (
          <span className="field-columns__toggles">
            <button type="button" className="field-columns__toggle" aria-label={t('documentStyle.boldOf', { name: name(field) })} title={t('documentStyle.boldOf', { name: name(field) })} aria-pressed={line.bold ?? false} onClick={() => setLine(place, { bold: !line.bold })}>
              <b aria-hidden="true">B</b>
            </button>
            <button type="button" className="field-columns__toggle" aria-label={t('documentStyle.italicOf', { name: name(field) })} title={t('documentStyle.italicOf', { name: name(field) })} aria-pressed={line.italic ?? false} onClick={() => setLine(place, { italic: !line.italic })}>
              <i aria-hidden="true">I</i>
            </button>
            <button type="button" className="field-columns__toggle" aria-label={t('documentStyle.labelOf', { name: name(field) })} title={t('documentStyle.labelOf', { name: name(field) })} aria-pressed={line.label ?? false} onClick={() => setLine(place, { label: !line.label })}>
              <span aria-hidden="true">Aa:</span>
            </button>
          </span>
        )}
      </li>
    )
  }

  const column = (title: string, columnIndex: number, children: React.ReactNode, length: number) => (
    <div className="field-columns__column" key={title}>
      <p className="field-columns__title" aria-hidden="true">{title}</p>
      <ul className="field-columns__list" aria-label={title} onDragOver={(e) => e.preventDefault()} onDrop={drop({ column: columnIndex, index: length })}>
        {children}
      </ul>
    </div>
  )

  return (
    <div className={`field-columns field-columns--${columns.length + 1}`}>
      <p id={helpId} className="sr-only">{t('documentStyle.moveHelp')}</p>
      {columns.map((lines, ci) => column(titles[ci] ?? '', ci, lines.map((l, li) => item(l.field, { column: ci, index: li }, lines.length, l)), lines.length))}
      {column(t('documentStyle.hide'), hiddenColumn, hidden.map((f, i) => item(f, { column: hiddenColumn, index: i }, hidden.length)), hidden.length)}
    </div>
  )
}
