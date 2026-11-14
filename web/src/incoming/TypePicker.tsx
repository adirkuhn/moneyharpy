import { type KeyboardEvent, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FormRow } from '../forms/Form'
import type { Confidence } from '../forms/Remark'
import { CaretIcon } from '../layout/icons'
import { type TypeChoice, typeChoices } from './queries'

type Props = {
  value: TypeChoice | null
  onChange: (type: TypeChoice) => void
  disabled?: boolean
  errors?: string[]
  /** Scan and Recognise chose the type; choosing the type it has confirms it. */
  remark?: Confidence
  /** The types it cannot become now, shown disabled with why: a booked purchase invoice only becomes a receipt, and back. */
  unavailable?: TypeChoice[]
}

/**
 * Moneybird's Type select of a document: each type with what it is for, or
 * why it cannot be chosen now. An upload without a type asks for one.
 */
export function TypePicker({ value, onChange, disabled = false, errors, remark, unavailable = [] }: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const list = useRef<HTMLUListElement>(null)
  const name = (type: TypeChoice) => t(`incoming.types.${type}`)
  const text = (type: TypeChoice) => t(`incoming.new.typeText.${type}`)
  const off = (type: TypeChoice) => unavailable.includes(type)

  useEffect(() => {
    if (!open) {
      return
    }
    list.current?.focus()
    const onClick = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('click', onClick)
    return () => document.removeEventListener('click', onClick)
  }, [open])

  const show = () => {
    setActive(Math.max(value ? typeChoices.indexOf(value) : 0, 0))
    setOpen(true)
  }
  const close = () => {
    setOpen(false)
    button.current?.focus()
  }
  const pick = (type: TypeChoice | undefined) => {
    if (!type || off(type)) {
      return
    }
    close()
    if (type !== value || remark) {
      onChange(type)
    }
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((i) => (e.key === 'ArrowDown' ? Math.min(i + 1, typeChoices.length - 1) : Math.max(i - 1, 0)))
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      pick(typeChoices[active])
    } else if (e.key === 'Escape' || e.key === 'Tab') {
      e.preventDefault()
      close()
    }
  }

  return (
    <FormRow label={t('incoming.type')} errors={errors} remark={remark}>
      {({ labelId, id, ...control }) => (
        <div className="picker type-picker" ref={root}>
          <button
            ref={button}
            type="button"
            role="combobox"
            className="picker__button type-picker__button"
            id={id}
            aria-labelledby={labelId}
            aria-expanded={open}
            aria-haspopup="listbox"
            aria-controls={`${id}-list`}
            disabled={disabled}
            {...control}
            onClick={() => (open ? close() : show())}
            onKeyDown={(e) => {
              if (!open && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
                e.preventDefault()
                show()
              }
            }}
          >
            <span className={`picker__value${value ? '' : ' is-placeholder'}`}>{value ? name(value) : t('incoming.view.chooseType')}</span>
            {!disabled && <CaretIcon />}
          </button>
          {open && (
            <ul
              ref={list}
              id={`${id}-list`}
              role="listbox"
              tabIndex={-1}
              className="picker__panel type-picker__list"
              aria-labelledby={labelId}
              aria-activedescendant={`${id}-${typeChoices[active]}`}
              onKeyDown={onKeyDown}
            >
              {typeChoices.map((type, i) => (
                <li
                  key={type}
                  id={`${id}-${type}`}
                  role="option"
                  aria-selected={type === value}
                  aria-disabled={off(type) || undefined}
                  className={`type-picker__option${i === active ? ' is-active' : ''}${off(type) ? ' is-disabled' : ''}`}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => pick(type)}
                >
                  <span className="type-picker__name">{name(type)}</span>
                  <span className="type-picker__text">{off(type) ? t('incoming.view.onlyNew') : text(type)}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </FormRow>
  )
}
