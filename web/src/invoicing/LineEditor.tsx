import { useTranslation } from 'react-i18next'
import type { Line } from './queries'

type Props = {
  /** Names the group, such as "Invoices: Sender block". */
  title: string
  fields: readonly string[]
  lines: Line[]
  onChange: (lines: Line[]) => void
}

/** Edits an ordered list of document lines: which field, bold, labelled. */
export function LineEditor({ title, fields, lines, onChange }: Props) {
  const { t } = useTranslation()
  const set = (i: number, change: Partial<Line>) => onChange(lines.map((l, j) => (j === i ? { ...l, ...change } : l)))
  const move = (i: number, by: number) => {
    const next = [...lines]
    ;[next[i], next[i + by]] = [next[i + by] as Line, next[i] as Line]
    onChange(next)
  }
  return (
    <fieldset className="line-editor" aria-label={title}>
      <legend className="line-editor__title">{title}</legend>
      {lines.length === 0 && <p className="empty">{t('documentStyle.noLines')}</p>}
      <ol className="line-editor__list">
        {lines.map((line, i) => (
          <li className="line-editor__row" key={i}>
            <label className="line-editor__field">
              <span className="sr-only">{t('documentStyle.field')}</span>
              <select value={line.field} onChange={(e) => set(i, { field: e.target.value })}>
                {fields.map((f) => <option key={f} value={f}>{t(`documentStyle.fields.${f}`)}</option>)}
              </select>
            </label>
            <label className="check">
              <input type="checkbox" checked={line.bold ?? false} onChange={(e) => set(i, { bold: e.target.checked })} />
              <span>{t('documentStyle.bold')}</span>
            </label>
            <label className="check">
              <input type="checkbox" checked={line.label ?? false} onChange={(e) => set(i, { label: e.target.checked })} />
              <span>{t('documentStyle.label')}</span>
            </label>
            <span className="line-editor__buttons">
              <button type="button" className="icon-btn" aria-label={t('documentStyle.lineUp', { n: i + 1 })} disabled={i === 0} onClick={() => move(i, -1)}>↑</button>
              <button type="button" className="icon-btn" aria-label={t('documentStyle.lineDown', { n: i + 1 })} disabled={i === lines.length - 1} onClick={() => move(i, 1)}>↓</button>
              <button type="button" className="icon-btn" aria-label={t('documentStyle.removeLine', { n: i + 1 })} onClick={() => onChange(lines.filter((_, j) => j !== i))}>&times;</button>
            </span>
          </li>
        ))}
      </ol>
      {lines.length < 20 && (
        <button type="button" className="icon-link icon-link--button" onClick={() => onChange([...lines, { field: fields[0] ?? '', bold: false, label: false }])}>
          {t('documentStyle.addLine')}
        </button>
      )}
    </fieldset>
  )
}
