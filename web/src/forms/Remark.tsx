import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { BoltIcon, WarningIcon } from '../layout/icons'

/** How sure Scan and Recognise was of a value it filled in. */
export type Confidence = 'certain' | 'uncertain'

/** The earlier document a remembered value came from: where it is, and its reference. */
export type RemarkSource = { to: string; reference?: string | null }

type Props = {
  confidence: Confidence | undefined
  /** In place of the tooltip of a filled in value. */
  label?: string
  /** The earlier document a remembered value came from, which the tooltip links to. */
  source?: RemarkSource
}

/**
 * The mark at the right of a field Scan and Recognise filled in, after
 * Moneybird's remark icons: a grey bolt when it read the value, a yellow
 * warning triangle when it asks to check it. The tooltip says which; for a
 * value remembered of an earlier document it links to that document.
 */
export function Remark({ confidence, label, source }: Props) {
  const { t } = useTranslation()
  if (!confidence) {
    return null
  }
  const icon = confidence === 'uncertain' ? <WarningIcon /> : <BoltIcon />
  if (source) {
    const name = source.reference || t('recognition.earlierDocument')
    const [before, after] = [t('recognition.basedOn.before'), t('recognition.basedOn.after')]
    return (
      // Focusable, so the tooltip with its link opens from the keyboard too.
      <span className={`remark remark--${confidence} remark--source`} tabIndex={0}>
        <span className="remark__icon" role="img" aria-label={before + name + after}>{icon}</span>
        <span className="remark__tip" role="tooltip">
          {before}
          {/* The remark may sit on something that opens on a click, such as a document's details. */}
          <Link to={source.to} onClick={(e) => e.stopPropagation()}>{name}</Link>
          {after}
        </span>
      </span>
    )
  }
  const text = label ?? t(confidence === 'uncertain' ? 'recognition.uncertain' : 'recognition.filled')
  return (
    <span className={`remark remark--${confidence}`} role="img" aria-label={text} title={text}>
      {icon}
    </span>
  )
}

/** The least sure of some values: one unsure value makes them all uncertain. */
export function leastSure(values: (Confidence | undefined)[]): Confidence | undefined {
  if (values.includes('uncertain')) {
    return 'uncertain'
  }
  return values.includes('certain') ? 'certain' : undefined
}
