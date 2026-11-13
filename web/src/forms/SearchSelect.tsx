import { type KeyboardEvent, useEffect, useId, useMemo, useRef, useState } from 'react'
import { CaretIcon, ListIcon } from '../layout/icons'

export type SearchOption = {
  value: string
  label: string
  /** Also searched, such as a category's code. */
  code?: string | null
  /** How far the option is indented, as a sub-category. */
  depth?: number
  /** A line under the option while nothing is searched. */
  separatorAfter?: boolean
}

/** Options under a heading; a group without a label has none. */
export type SearchGroup = { label?: string; options: SearchOption[] }

type Props = {
  /** The name of the select, for screen readers; or labelledBy. */
  label?: string
  labelledBy?: string
  id?: string
  /** The search field's placeholder, such as "Find category". */
  searchLabel: string
  /** The row shown when the search finds nothing, such as "No category found". */
  emptyLabel: string
  value: string
  options?: SearchOption[]
  groups?: SearchGroup[]
  /** A group shown first while nothing is searched, such as the most used. */
  pinned?: SearchGroup
  /** The row at the bottom to add an option, such as "Add Category...". */
  add?: { label: string; onAdd: () => void }
  /** Shown while no option has the value. */
  placeholder?: string
  onChange: (value: string) => void
  className?: string
  'aria-invalid'?: boolean
  'aria-describedby'?: string
}

/** Compares text without case or accents. */
const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()

/** Whether an option's label or code holds the search. */
export function matches(option: SearchOption, query: string) {
  const needle = fold(query.trim())
  return needle === '' || fold(option.label).includes(needle) || fold(option.code ?? '').includes(needle)
}

type Entry = { option?: SearchOption; add?: true }

/**
 * A select whose options can be searched, after Moneybird's select popups: the
 * chosen value shows like a select; opened, a search field over the options,
 * grouped under headings, and at the bottom a row to add a new one.
 */
export function SearchSelect(props: Props) {
  const { value, searchLabel, emptyLabel, add, pinned, onChange } = props
  const ownId = useId()
  const id = props.id ?? ownId
  const listId = `${id}-list`
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const search = useRef<HTMLInputElement>(null)

  const all = useMemo(() => [...(props.options ? [{ options: props.options }] : []), ...(props.groups ?? [])], [props.options, props.groups])
  const shown = useMemo(() => {
    const found = all.map((g) => ({ ...g, options: g.options.filter((o) => matches(o, query)) })).filter((g) => g.options.length > 0)
    return query.trim() === '' && pinned && pinned.options.length > 0 ? [pinned, ...found] : found
  }, [all, pinned, query])
  const entries: Entry[] = [...shown.flatMap((g) => g.options.map((option) => ({ option }))), ...(add ? [{ add: true as const }] : [])]
  const optionCount = entries.length - (add ? 1 : 0)
  const chosen = all.flatMap((g) => g.options).find((o) => o.value === value)
  const entryId = (i: number) => `${id}-option-${i}`

  useEffect(() => {
    if (!open) {
      return
    }
    search.current?.focus()
    const onDown = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) {
        setOpen(false)
        setQuery('')
      }
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  useEffect(() => {
    if (open) {
      document.getElementById(entryId(active))?.scrollIntoView?.({ block: 'nearest' })
    }
  })

  const openWith = (text: string) => {
    setQuery(text)
    // The keyboard starts at the chosen option, else at the first.
    const at = text ? 0 : entries.findIndex((e) => e.option?.value === value && !pinned?.options.includes(e.option))
    setActive(Math.max(at, 0))
    setOpen(true)
  }
  const close = (focus = true) => {
    setOpen(false)
    setQuery('')
    if (focus) {
      button.current?.focus()
    }
  }
  const pick = (entry: Entry | undefined) => {
    if (entry?.add) {
      close()
      add?.onAdd()
    } else if (entry?.option) {
      onChange(entry.option.value)
      close()
    }
  }

  const onTriggerKey = (e: KeyboardEvent) => {
    if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) {
      e.preventDefault()
      openWith('')
    } else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault()
      openWith(e.key)
    }
  }
  const onSearchKey = (e: KeyboardEvent) => {
    const last = entries.length - 1
    const moves: Record<string, (i: number) => number> = {
      ArrowDown: (i) => Math.min(i + 1, last),
      ArrowUp: (i) => Math.max(i - 1, 0),
      Home: () => 0,
      End: () => last,
    }
    const move = moves[e.key]
    if (move) {
      e.preventDefault()
      setActive(move)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      pick(entries[active])
    } else if (e.key === 'Escape') {
      // Only the select closes, not a popup it is in.
      e.preventDefault()
      e.stopPropagation()
      close()
    } else if (e.key === 'Tab') {
      close(false)
    }
  }

  let index = -1
  const row = (option: SearchOption, separator: boolean) => {
    index += 1
    const i = index
    return (
      <div
        key={`${i}-${option.value}`}
        id={entryId(i)}
        role="option"
        aria-selected={option.value === value}
        className={`search-select__option${i === active ? ' is-active' : ''}${separator && option.separatorAfter ? ' has-separator' : ''}`}
        style={option.depth ? { paddingInlineStart: `calc(var(--indent) + ${option.depth}rem)` } : undefined}
        onMouseDown={(e) => e.preventDefault()}
        onMouseMove={() => setActive(i)}
        onClick={() => pick({ option })}
      >
        {option.label}
      </div>
    )
  }

  return (
    <div className="search-select" ref={root}>
      <button
        ref={button}
        id={id}
        type="button"
        role="combobox"
        className={`picker__button search-select__button${props.className ? ` ${props.className}` : ''}`}
        aria-label={props.label}
        aria-labelledby={props.labelledBy}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listId}
        aria-invalid={props['aria-invalid'] || undefined}
        aria-describedby={props['aria-describedby']}
        onClick={() => (open ? close() : openWith(''))}
        onKeyDown={onTriggerKey}
      >
        <span className={`picker__value${chosen ? '' : ' is-placeholder'}`}>{chosen?.label ?? props.placeholder ?? ''}</span>
        <CaretIcon />
      </button>
      {open && (
        <div className="search-select__panel">
          <input
            ref={search}
            type="search"
            className="search-select__search"
            aria-label={searchLabel}
            placeholder={searchLabel}
            autoComplete="off"
            aria-autocomplete="list"
            aria-controls={listId}
            aria-activedescendant={entries[active] ? entryId(active) : undefined}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setActive(0)
            }}
            onKeyDown={onSearchKey}
          />
          <div id={listId} role="listbox" className="search-select__list" aria-label={props.label} aria-labelledby={props.labelledBy}>
            {shown.map((g, gi) => {
              const rows = g.options.map((o) => row(o, query.trim() === ''))
              if (!g.label) {
                return rows
              }
              const headingId = `${id}-group-${gi}`
              return (
                <div key={headingId} role="group" aria-labelledby={headingId} className="search-select__group">
                  <div id={headingId} role="presentation" className="search-select__heading">{g.label}</div>
                  {rows}
                </div>
              )
            })}
            {optionCount === 0 && <div role="presentation" className="search-select__empty">{emptyLabel}</div>}
            {add && (
              <div
                id={entryId(entries.length - 1)}
                role="option"
                aria-selected={false}
                className={`search-select__option search-select__add${active === entries.length - 1 ? ' is-active' : ''}`}
                onMouseDown={(e) => e.preventDefault()}
                onMouseMove={() => setActive(entries.length - 1)}
                onClick={() => pick({ add: true })}
              >
                <ListIcon kind="plus" />
                <span>{add.label}</span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
