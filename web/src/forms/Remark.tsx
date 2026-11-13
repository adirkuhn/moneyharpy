import { useTranslation } from 'react-i18next'
import { BoltIcon } from '../layout/icons'

/** How sure Scan and Recognise was of a value it filled in. */
export type Confidence = 'certain' | 'uncertain'

/**
 * The bolt at the right of a field Scan and Recognise filled in, after
 * Moneybird's remark icon: grey when it read the value, gold when it asks to
 * check it. The tooltip says which.
 */
export function Remark({ confidence, label }: { confidence: Confidence | undefined; /** In place of the tooltip of a filled in value. */ label?: string }) {
  const { t } = useTranslation()
  if (!confidence) {
    return null
  }
  const text = label ?? t(confidence === 'uncertain' ? 'recognition.uncertain' : 'recognition.filled')
  return (
    <span className={`remark remark--${confidence}`} role="img" aria-label={text} title={text}>
      <BoltIcon />
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
