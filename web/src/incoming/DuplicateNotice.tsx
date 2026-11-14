import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import type { components } from '../api/schema'
import { formatAmount } from '../banking/queries'
import { WarningIcon } from '../layout/icons'
import { listDate } from '../list/DocumentTable'
import { documentPath, hasAmount } from './documents'
import { choiceOf } from './queries'

export type PossibleDuplicate = components['schemas']['PossibleDuplicate']

/** What a possible duplicate is called in a link: its type and reference, as the page title names it. */
function useName() {
  const { t } = useTranslation()
  return (d: PossibleDuplicate) => {
    const choice = choiceOf[d.type]
    const type = choice ? t(`incoming.types.${choice}`) : t('incoming.view.document')
    return d.reference ? `${type} ${d.reference}` : type
  }
}

type Props = { administrationId: string; duplicates: PossibleDuplicate[]; currency: string }

/**
 * Warns that the document may be a copy of others, with a link to each and
 * why it looks alike. A warning only: the document can be saved as it is.
 */
export function DuplicateNotice({ administrationId, duplicates, currency }: Props): ReactNode {
  const { t, i18n } = useTranslation()
  const name = useName()
  const [first] = duplicates
  if (!first) {
    return null
  }
  const item = (d: PossibleDuplicate) => (
    <>
      <Link to={documentPath(administrationId, d)}>{name(d)}</Link>
      {' '}
      <span className="duplicate-notice__facts">
        ({[listDate(d.date), ...(hasAmount(d.type) ? [formatAmount(d.total_price_incl_tax, currency, i18n.language)] : [])].join(', ')}
        {' — '}
        {d.reasons.map((r) => t(`incoming.duplicates.reasons.${r}`)).join(', ')})
      </span>
    </>
  )
  return (
    <div className="alert duplicate-notice" role="note" aria-label={t('incoming.duplicates.title')}>
      <WarningIcon />
      {duplicates.length === 1 ? (
        <p className="duplicate-notice__text">{t('incoming.duplicates.of')} {item(first)}</p>
      ) : (
        <div className="duplicate-notice__text">
          <p>{t('incoming.duplicates.ofMany')}</p>
          <ul>
            {duplicates.map((d) => <li key={d.id}>{item(d)}</li>)}
          </ul>
        </div>
      )}
    </div>
  )
}

/** The mark on a row of the Incoming list of a document that may be a copy of another. */
export function DuplicateMark({ count }: { count: number }): ReactNode {
  const { t } = useTranslation()
  if (count === 0) {
    return null
  }
  // The space keeps the mark a word of its own in the name of the row's link.
  return (
    <>
      {' '}
      {/* Just the icon, so a long reference keeps its width; the words are its name and tooltip. */}
      <span className="duplicate-mark" role="img" aria-label={t('incoming.duplicates.mark')} title={t('incoming.duplicates.title')}>
        <WarningIcon />
      </span>
    </>
  )
}
