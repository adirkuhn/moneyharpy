import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { CaretIcon } from '../layout/icons'

export type SelectOption = { value: string; label: string; separatorAfter?: boolean }

type Props = {
  label: string
  searchLabel: string
  emptyLabel: string
  value: string
  options: SelectOption[]
  onChange: (value: string) => void
}

/** A select whose long list can be narrowed by typing. */
export function SearchableSelect({ label, searchLabel, emptyLabel, value, options, onChange }: Props) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const id = useId()

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return needle ? options.filter((o) => o.label.toLowerCase().includes(needle)) : options
  }, [options, query])

  useEffect(() => {
    if (!open) {
      return
    }
    const onClick = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('click', onClick)
    return () => document.removeEventListener('click', onClick)
  }, [open])

  const close = () => {
    setOpen(false)
    setQuery('')
    setActive(0)
    button.current?.focus()
  }
  const pick = (option: SelectOption | undefined) => {
    if (option) {
      onChange(option.value)
      close()
    }
  }
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((i) => Math.min(i + 1, visible.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      pick(visible[active])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      close()
    }
  }

  return (
    <div className="field" ref={root}>
      <span id={`${id}-label`}>{label}</span>
      <div className="picker">
        <button
          ref={button}
          type="button"
          role="combobox"
          className="picker__button"
          aria-labelledby={`${id}-label`}
          aria-expanded={open}
          aria-haspopup="listbox"
          aria-controls={`${id}-list`}
          onClick={() => setOpen(!open)}
        >
          <span className="picker__value">{options.find((o) => o.value === value)?.label ?? ''}</span>
          <CaretIcon />
        </button>
        {open && (
          <div className="picker__panel">
            <input
              type="search"
              aria-label={searchLabel}
              placeholder={searchLabel}
              autoFocus
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setActive(0)
              }}
              onKeyDown={onKeyDown}
            />
            <ul id={`${id}-list`} role="listbox" aria-labelledby={`${id}-label`}>
              {visible.map((option, index) => (
                <li
                  key={option.value}
                  role="option"
                  aria-selected={option.value === value}
                  className={`${index === active ? 'is-active' : ''}${option.separatorAfter && !query ? ' has-separator' : ''}`}
                  onClick={() => pick(option)}
                >
                  {option.label}
                </li>
              ))}
            </ul>
            {visible.length === 0 && <p className="picker__empty">{emptyLabel}</p>}
          </div>
        )}
      </div>
    </div>
  )
}
