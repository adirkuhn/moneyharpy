import { Fragment, type ReactNode, useId, useLayoutEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { parseTextile, renderTags, tagGroups, type TagGroup, type TagValues } from './textile'
import './textile.css'

/** An extra tag, such as one per custom field of the sender, with its name. */
export type ExtraTag = { group: TagGroup; name: string; label: string }

type Props = {
  label: string
  value: string
  onChange: (value: string) => void
  /** Beside the label, such as Moneybird's "Edit in workflow" link. */
  labelExtra?: ReactNode
  extraTags?: ExtraTag[]
  maxLength?: number
  errors?: string[]
  autoFocus?: boolean
}

/**
 * Moneybird's textile editor: buttons that wrap the selection in _italic_,
 * *bold* or +underline+, a {tag} menu that puts a tag at the cursor, and a
 * text area that grows with its text.
 */
export function TextileEditor({ label, value, onChange, labelExtra, extraTags = [], maxLength, errors, autoFocus }: Props) {
  const { t } = useTranslation()
  const id = useId()
  const area = useRef<HTMLTextAreaElement>(null)
  const invalid = !!errors?.length

  useLayoutEffect(() => {
    const el = area.current
    if (el) {
      el.style.height = 'auto'
      el.style.height = `${el.scrollHeight + 2}px`
    }
  }, [value])

  /** Changes the text and puts the selection where it belongs afterwards. */
  const edit = (next: string, start: number, end: number) => {
    onChange(maxLength ? next.slice(0, maxLength) : next)
    requestAnimationFrame(() => {
      area.current?.focus()
      area.current?.setSelectionRange(start, end)
    })
  }
  const wrap = (marker: string) => {
    const el = area.current
    const start = el?.selectionStart ?? value.length
    const end = el?.selectionEnd ?? value.length
    edit(value.slice(0, start) + marker + value.slice(start, end) + marker + value.slice(end), start + 1, end + 1)
  }
  const insert = (tag: string) => {
    const el = area.current
    const start = el?.selectionStart ?? value.length
    const end = el?.selectionEnd ?? value.length
    edit(value.slice(0, start) + tag + value.slice(end), start + tag.length, start + tag.length)
  }

  return (
    <div className={`textile${invalid ? ' is-invalid' : ''}`}>
      <div className="textile__header">
        <label className="textile__label" htmlFor={id}>{label}</label>
        {labelExtra}
      </div>
      <div className="textile__toolbar" role="toolbar" aria-label={t('textile.toolbar')} aria-controls={id}>
        <button type="button" className="textile__button" aria-label={t('textile.italic')} title={t('textile.italic')} onClick={() => wrap('_')}><em>{t('textile.italic')}</em></button>
        <button type="button" className="textile__button" aria-label={t('textile.bold')} title={t('textile.bold')} onClick={() => wrap('*')}><strong>{t('textile.bold')}</strong></button>
        <button type="button" className="textile__button" aria-label={t('textile.underline')} title={t('textile.underline')} onClick={() => wrap('+')}><u>{t('textile.underline')}</u></button>
        <select
          className="textile__tags"
          aria-label={t('textile.insertTag')}
          value=""
          onChange={(e) => {
            if (e.target.value) {
              insert(e.target.value)
            }
          }}
        >
          <option value="">{'{tag}'}</option>
          {(Object.keys(tagGroups) as TagGroup[]).map((group) => (
            <optgroup key={group} label={t(`textile.groups.${group}`)}>
              {[
                ...tagGroups[group].map((name) => ({ name, label: t(`textile.tags.${group}.${name}`) })),
                ...extraTags.filter((x) => x.group === group),
              ]
                .sort((a, b) => a.label.localeCompare(b.label))
                .map((tag) => <option key={tag.name} value={`{${group}.${tag.name}}`}>{tag.label}</option>)}
            </optgroup>
          ))}
        </select>
      </div>
      <textarea
        ref={area}
        id={id}
        className="textile__area"
        rows={3}
        autoFocus={autoFocus}
        maxLength={maxLength}
        value={value}
        aria-invalid={invalid || undefined}
        aria-describedby={invalid ? `${id}-errors` : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
      {invalid && <p id={`${id}-errors`} className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
    </div>
  )
}

/** A textile text as it is shown: styled, with its tags filled in and its line breaks kept. */
export function TextileText({ text, values = {} }: { text: string; values?: TagValues }) {
  const lines = text.split('\n')
  return (
    <>
      {lines.map((line, i) => (
        <Fragment key={i}>
          {i > 0 && <br />}
          {parseTextile(line).map((s, j) => {
            let node: ReactNode = renderTags(s.text, values)
            if (s.underline) node = <u>{node}</u>
            if (s.italic) node = <em>{node}</em>
            if (s.bold) node = <strong>{node}</strong>
            return <Fragment key={j}>{node}</Fragment>
          })}
        </Fragment>
      ))}
    </>
  )
}
